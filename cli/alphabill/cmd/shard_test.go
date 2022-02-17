package cmd

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
					HomeDir:    "/custom-home",
					CfgFile:    "/custom-home" + string(os.PathSeparator) + defaultConfigFile,
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		}, {
			args: "shard --home=/custom-home --config=custom-config.props",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir:    "/custom-home",
					CfgFile:    "/custom-home/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		}, {
			args: "shard --config=custom-config.props",
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir:    defaultHomeDir,
					CfgFile:    defaultHomeDir + "/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
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
				sc.Server = &grpcServerConfiguration{
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
		}, {
			args: "shard --home=/custom-home-1",
			envVars: []envVar{
				{"AB_HOME", "/custom-home-2"},
				{"AB_CONFIG", "custom-config.props"},
				{"AB_LOGGER_CONFIG", "custom-log-conf.yaml"},
			},
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir:    "/custom-home-1",
					CfgFile:    "/custom-home-1/custom-config.props",
					LogCfgFile: "custom-log-conf.yaml",
				}
				return sc
			}(),
		}, {
			args: "shard",
			envVars: []envVar{
				{"AB_HOME", "/custom-home"},
				{"AB_CONFIG", "custom-config.props"},
			},
			expectedConfig: func() *shardConfiguration {
				sc := defaultShardConfiguration()
				sc.Root = &rootConfiguration{
					HomeDir:    "/custom-home",
					CfgFile:    "/custom-home/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
				}
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
logger-config=custom-log-conf.yaml
`

	// Write temporary file and clean up afterwards
	f, err := os.CreateTemp("", "example-conf.*.props")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err = f.Write([]byte(configFileContents)); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}

	expectedConfig := defaultShardConfiguration()
	expectedConfig.Root.CfgFile = f.Name()
	expectedConfig.Root.LogCfgFile = "custom-log-conf.yaml"
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
			HomeDir:    defaultHomeDir,
			CfgFile:    defaultHomeDir + string(os.PathSeparator) + defaultConfigFile,
			LogCfgFile: defaultLoggerConfigFile,
		},
		Server: &grpcServerConfiguration{
			Address:        defaultServerAddr,
			MaxRecvMsgSize: defaultMaxRecvMsgSize,
		},
		InitialBillValue:   defaultInitialBillValue,
		DCMoneySupplyValue: defaultDCMoneySupplyValue,
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
		transactionsClient := transaction.NewTransactionsClient(conn)

		// Test cases
		makeSuccessfulPayment(t, ctx, transactionsClient)
		makeFailingPayment(t, ctx, transactionsClient)

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func makeSuccessfulPayment(t *testing.T, ctx context.Context, txClient transaction.TransactionsClient) {
	initialBillID := uint256.NewInt(defaultInitialBillId).Bytes32()

	tx := &transaction.Transaction{
		UnitId:                initialBillID[:],
		TransactionAttributes: new(anypb.Any),
		Timeout:               1,
		OwnerProof:            []byte{script.StartByte},
	}
	bt := &transaction.BillTransfer{
		NewBearer:   script.PredicateAlwaysTrue(),
		TargetValue: defaultInitialBillValue,
		Backlink:    nil,
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, bt, proto.MarshalOptions{})
	require.NoError(t, err)

	response, err := txClient.ProcessTransaction(ctx, tx)
	require.NoError(t, err)
	require.True(t, response.Ok, "Successful response ok should be true")
}

func makeFailingPayment(t *testing.T, ctx context.Context, txClient transaction.TransactionsClient) {
	wrongBillID := uint256.NewInt(6).Bytes32()

	tx := &transaction.Transaction{
		UnitId:                wrongBillID[:],
		TransactionAttributes: new(anypb.Any),
		Timeout:               0,
		OwnerProof:            []byte{script.StartByte},
	}
	bt := &transaction.BillTransfer{
		NewBearer:   script.PredicateAlwaysTrue(),
		TargetValue: defaultInitialBillValue,
		Backlink:    nil,
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, bt, proto.MarshalOptions{})
	require.NoError(t, err)

	response, err := txClient.ProcessTransaction(ctx, tx)
	require.Error(t, err)
	require.Nil(t, response, "Failing payment should not return response")
}
