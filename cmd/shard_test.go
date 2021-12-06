package cmd

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/payment"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"google.golang.org/grpc"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
)

type envVar [2]string

func TestShardConfig_EnvAndFlags(t *testing.T) {
	tests := []struct {
		args           string   // arguments as a space separated string
		envVars        []envVar // Environment variables that will be set before creating command
		expectedConfig *shardConfiguration
	}{
		// Root configuration permutations
		{
			args:           "shard",
			expectedConfig: defaultShardConfiguration(),
		}, {
			args: "shard --home=/custom-home",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir: "/custom-home",
					CfgFile: "/custom-home" + string(os.PathSeparator) + defaultConfigFile,
				}
				return sc
			}(),
		}, {
			args: "shard --home=/custom-home --config=custom-config.props",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir: "/custom-home",
					CfgFile: "/custom-home/custom-config.props",
				}
				return sc
			}(),
		}, {
			args: "shard --config=custom-config.props",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir: defaultHomeDir,
					CfgFile: defaultHomeDir + "/custom-config.props",
				}
				return sc
			}(),
		},
		// Shard configuration from flags
		{
			args: "shard --initial-bill-value=555",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.InitialBillValue = 555
				return sc
			}(),
		}, {
			args: "shard --server-address=srv:1234 --server-max-recv-msg-size=66 --server-max-connection-age-ms=77 --server-max-connection-age-grace-ms=88",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Server = &serverConfiguration{
					Address:                 "srv:1234",
					MaxRecvMsgSize:          66,
					MaxConnectionAgeMs:      77,
					MaxConnectionAgeGraceMs: 88,
				}
				return sc
			}(),
		},
		// Shard configuration from ENV
		{
			args: "shard",
			envVars: []envVar{
				{"AB_INITIAL_BILL_VALUE", "555"},
				{"AB_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.InitialBillValue = 555
				sc.Server.Address = "srv:1234"
				return sc
			}(),
		}, {
			args: "shard --initial-bill-value=666 --server-address=srv:666",
			envVars: []envVar{
				{"AB_INITIAL_BILL_VALUE", "555"},
				{"AB_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.InitialBillValue = 666
				sc.Server.Address = "srv:666"
				return sc
			}(),
		},
	}
	for _, tt := range tests {
		t.Run("shard_conf|"+tt.args+"|"+envVarsStr(tt.envVars), func(t *testing.T) {
			var actualConfig *shardConfiguration
			shardRunFunc := func(ctx context.Context, sc *shardConfiguration) error {
				actualConfig = sc
				return nil
			}

			// Set environment variables only for single test.
			for _, en := range tt.envVars {
				err := os.Setenv(en[0], en[1])
				require.NoError(t, err)
				defer os.Unsetenv(en[0])
			}

			abApp := New()
			abApp.rootCmd.SetArgs(strings.Split(tt.args, " "))
			abApp.Execute(context.Background(), Opts.ShardRunFunc(shardRunFunc))

			assert.Equal(t, tt.expectedConfig, actualConfig)
		})
	}
}

func TestShardConfig_ConfigFile(t *testing.T) {
	configFileContents := `
initial-bill-value=666
server-address=srv:1234
server-max-recv-msg-size=9999
`

	// Write temporary file and clean up afterwards
	f, err := os.CreateTemp("", "example-conf.*.props")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write([]byte(configFileContents)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	expectedConfig := defaultShardConfiguration()
	expectedConfig.Root.CfgFile = f.Name()
	expectedConfig.InitialBillValue = 666
	expectedConfig.Server.Address = "srv:1234"
	expectedConfig.Server.MaxRecvMsgSize = 9999

	// Set up shard runner mock
	var actualConfig *shardConfiguration
	shardRunFunc := func(ctx context.Context, sc *shardConfiguration) error {
		actualConfig = sc
		return nil
	}

	abApp := New()
	args := "shard --config=" + f.Name()
	abApp.rootCmd.SetArgs(strings.Split(args, " "))
	abApp.Execute(context.Background(), Opts.ShardRunFunc(shardRunFunc))

	assert.Equal(t, expectedConfig, actualConfig)
}

func defaultShardConfiguration() *shardConfiguration {
	return &shardConfiguration{
		Root: &rootConfiguration{
			HomeDir: defaultHomeDir,
			CfgFile: defaultHomeDir + string(os.PathSeparator) + defaultConfigFile,
		},
		Server: &serverConfiguration{
			Address:        defaultServerAddr,
			MaxRecvMsgSize: defaultMaxRecvMsgSize,
		},
		InitialBillValue: defaultInitialBillValue,
	}
}

// envVarsStr creates sting for test names from envVars
func envVarsStr(envVars []envVar) (out string) {
	if len(envVars) == 0 {
		return
	}
	out += "ENV:"
	for i, ev := range envVars {
		if i > 0 {
			out += "&"
		}
		out += ev[0] + "=" + ev[1]
	}
	return
}

func TestRunShard_Ok(t *testing.T) {
	test.MustRunInTime(t, 5*time.Second, func() {
		port := "9543"
		listenAddr := ":" + port // listen is on all devices, so it would work in CI inside docker too.
		dialAddr := "localhost:" + port

		conf := defaultShardConfiguration()
		conf.Server.Address = listenAddr

		appStoppedWg := sync.WaitGroup{}
		ctx, _ := async.WithWaitGroup(context.Background())
		ctx, ctxCancel := context.WithCancel(ctx)

		// Starting the shard in background
		appStoppedWg.Add(1)
		go func() {
			err := defaultShardRunFunc(ctx, conf)
			require.NoError(t, err)
			appStoppedWg.Done()
		}()

		log.Info("Started shard and dialing...")
		// Create the gRPC client
		conn, err := grpc.DialContext(ctx, dialAddr, grpc.WithInsecure())
		require.NoError(t, err)
		defer conn.Close()
		paymentsClient := payment.NewPaymentsClient(conn)

		// Test cases
		makeSuccessfulPayment(t, ctx, paymentsClient)
		makeFailingPayment(t, ctx, paymentsClient)

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func makeSuccessfulPayment(t *testing.T, ctx context.Context, paymentsClient payment.PaymentsClient) {
	paymentResponse, err := paymentsClient.MakePayment(ctx, &payment.PaymentRequest{
		BillId:            1,
		PaymentType:       payment.PaymentRequest_TRANSFER,
		Amount:            0,
		PayeePredicate:    script.PredicateAlwaysTrue(),
		Backlink:          []byte{},
		PredicateArgument: []byte{script.StartByte},
	})
	require.NoError(t, err)
	require.NotEmpty(t, paymentResponse.PaymentId, "Successful payment should return some ID")
}

func makeFailingPayment(t *testing.T, ctx context.Context, paymentsClient payment.PaymentsClient) {
	paymentResponse, err := paymentsClient.MakePayment(ctx, &payment.PaymentRequest{
		BillId:            100,
		PaymentType:       payment.PaymentRequest_TRANSFER,
		Amount:            0,
		PayeePredicate:    script.PredicateAlwaysTrue(),
		Backlink:          []byte{},
		PredicateArgument: []byte{script.StartByte},
	})
	require.Error(t, err)
	require.Nil(t, paymentResponse, "Failing payment should return response")
}
