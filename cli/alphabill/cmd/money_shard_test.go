package cmd

import (
	"context"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	billtx "gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"google.golang.org/grpc/credentials/insecure"

	"google.golang.org/grpc"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type envVar [2]string

func TestShardConfig_EnvAndFlags(t *testing.T) {
	tests := []struct {
		args           string   // arguments as a space separated string
		envVars        []envVar // Environment variables that will be set before creating command
		expectedConfig *moneyShardConfiguration
	}{
		// Base configuration permutations
		{
			args:           "shard",
			expectedConfig: defaultShardConfiguration(),
		}, {
			args: "shard --home=/custom-home",
			expectedConfig: func() *moneyShardConfiguration {
				sc := defaultShardConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    "/custom-home",
					CfgFile:    path.Join("/custom-home", defaultConfigFile),
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		}, {
			args: "shard --home=/custom-home --config=custom-config.props",
			expectedConfig: func() *moneyShardConfiguration {
				sc := defaultShardConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    "/custom-home",
					CfgFile:    "/custom-home/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		}, {
			args: "shard --config=custom-config.props",
			expectedConfig: func() *moneyShardConfiguration {
				sc := defaultShardConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    alphabillHomeDir(),
					CfgFile:    alphabillHomeDir() + "/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		},
		// Shard configuration from flags
		{
			args: "shard --initial-bill-value=555",
			expectedConfig: func() *moneyShardConfiguration {
				sc := defaultShardConfiguration()
				sc.InitialBillValue = 555
				return sc
			}(),
		}, {
			args: "shard --server-address=srv:1234 --server-max-recv-msg-size=66 --server-max-connection-age-ms=77 --server-max-connection-age-grace-ms=88",
			expectedConfig: func() *moneyShardConfiguration {
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
			expectedConfig: func() *moneyShardConfiguration {
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
			expectedConfig: func() *moneyShardConfiguration {
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
			expectedConfig: func() *moneyShardConfiguration {
				sc := defaultShardConfiguration()
				sc.Base = &baseConfiguration{
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
			expectedConfig: func() *moneyShardConfiguration {
				sc := defaultShardConfiguration()
				sc.Base = &baseConfiguration{
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
			var actualConfig *moneyShardConfiguration
			shardRunFunc := func(ctx context.Context, sc *moneyShardConfiguration) error {
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
			abApp.baseCmd.SetArgs(strings.Split(tt.args, " "))
			abApp.WithOpts(Opts.ShardRunFunc(shardRunFunc)).Execute(context.Background())

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
	expectedConfig.Base.CfgFile = f.Name()
	expectedConfig.Base.LogCfgFile = "custom-log-conf.yaml"
	expectedConfig.InitialBillValue = 666
	expectedConfig.Server.Address = "srv:1234"
	expectedConfig.Server.MaxRecvMsgSize = 9999

	// Set up shard runner mock
	var actualConfig *moneyShardConfiguration
	shardRunFunc := func(ctx context.Context, sc *moneyShardConfiguration) error {
		actualConfig = sc
		return nil
	}

	abApp := New()
	args := "shard --config=" + f.Name()
	abApp.baseCmd.SetArgs(strings.Split(args, " "))
	abApp.WithOpts(Opts.ShardRunFunc(shardRunFunc)).Execute(context.Background())

	assert.Equal(t, expectedConfig, actualConfig)
}

func TestRunShard_Ok(t *testing.T) {
	test.MustRunInTime(t, 5*time.Second, func() {
		port := "9543"
		listenAddr := ":" + port // listen is on all devices, so it would work in CI inside docker too.
		dialAddr := "localhost:" + port

		conf := defaultShardConfiguration()
		conf.UnicityTrustBase = []string{"0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"}
		conf.Server.Address = listenAddr

		appStoppedWg := sync.WaitGroup{}
		ctx, _ := async.WithWaitGroup(context.Background())
		ctx, ctxCancel := context.WithCancel(ctx)

		// Starting the shard in background
		appStoppedWg.Add(1)
		go func() {
			err := defaultMoneyShardRunFunc(ctx, conf)
			require.NoError(t, err)
			appStoppedWg.Done()
		}()

		log.Info("Started shard and dialing...")
		// Create the gRPC client
		conn, err := grpc.DialContext(ctx, dialAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()
		rpcClient := alphabill.NewAlphabillServiceClient(conn)

		// Test cases
		makeSuccessfulPayment(t, ctx, rpcClient)
		makeFailingPayment(t, ctx, rpcClient)

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func defaultShardConfiguration() *moneyShardConfiguration {
	return &moneyShardConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: &baseConfiguration{
				HomeDir:    alphabillHomeDir(),
				CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
				LogCfgFile: defaultLoggerConfigFile,
			},
			Server: &grpcServerConfiguration{
				Address:        defaultServerAddr,
				MaxRecvMsgSize: defaultMaxRecvMsgSize,
			},
		},
		InitialBillValue:   defaultInitialBillValue,
		DCMoneySupplyValue: defaultDCMoneySupplyValue,
		UnicityTrustBase:   []string{},
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

func makeSuccessfulPayment(t *testing.T, ctx context.Context, txClient alphabill.AlphabillServiceClient) {
	initialBillID := uint256.NewInt(defaultInitialBillId).Bytes32()

	tx := &transaction.Transaction{
		UnitId:                initialBillID[:],
		TransactionAttributes: new(anypb.Any),
		Timeout:               1,
		OwnerProof:            script.PredicateArgumentEmpty(),
		SystemId:              []byte{0},
	}
	bt := &billtx.BillTransfer{
		NewBearer:   script.PredicateAlwaysTrue(),
		TargetValue: defaultInitialBillValue,
		Backlink:    nil,
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, bt, proto.MarshalOptions{})
	require.NoError(t, err)

	response, err := txClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
	require.NoError(t, err)
	require.True(t, response.Ok, "Successful response ok should be true")
}

func makeFailingPayment(t *testing.T, ctx context.Context, txClient alphabill.AlphabillServiceClient) {
	wrongBillID := uint256.NewInt(6).Bytes32()

	tx := &transaction.Transaction{
		UnitId:                wrongBillID[:],
		TransactionAttributes: new(anypb.Any),
		Timeout:               0,
		OwnerProof:            script.PredicateArgumentEmpty(),
		SystemId:              []byte{0},
	}
	bt := &billtx.BillTransfer{
		NewBearer:   script.PredicateAlwaysTrue(),
		TargetValue: defaultInitialBillValue,
		Backlink:    nil,
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, bt, proto.MarshalOptions{})
	require.NoError(t, err)

	response, err := txClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
	require.Error(t, err)
	require.Nil(t, response, "Failing payment should not return response")
}
