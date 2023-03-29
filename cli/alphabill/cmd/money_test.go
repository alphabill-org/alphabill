package cmd

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	test "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type envVar [2]string

func TestMoneyNodeConfig_EnvAndFlags(t *testing.T) {
	tests := []struct {
		args           string   // arguments as a space separated string
		envVars        []envVar // Environment variables that will be set before creating command
		expectedConfig *moneyNodeConfiguration
	}{
		// Base configuration permutations
		{
			args:           "money",
			expectedConfig: defaultMoneyNodeConfiguration(),
		}, {
			args: "money --home=/custom-home",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    "/custom-home",
					CfgFile:    filepath.Join("/custom-home", defaultConfigFile),
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		}, {
			args: "money --home=/custom-home --config=custom-config.props",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    "/custom-home",
					CfgFile:    "/custom-home/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		}, {
			args: "money --config=custom-config.props",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    alphabillHomeDir(),
					CfgFile:    alphabillHomeDir() + "/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		},
		// Validator configuration from flags
		{
			args: "money -a=/ip4/127.0.0.1/tcp/21111 -p 1=/ip4/127.0.0.1/tcp/21111,2=/ip4/127.0.0.1/tcp/21112 -g genesis.json -k keys.json -r /ip4/127.0.0.1/tcp/26611 --ledger-replication-max-blocks=10 --ledger-replication-max-transactions=99",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				peers := make(map[string]string)
				peers["1"] = "/ip4/127.0.0.1/tcp/21111"
				peers["2"] = "/ip4/127.0.0.1/tcp/21112"

				sc.Node = &startNodeConfiguration{
					Address:                    "/ip4/127.0.0.1/tcp/21111",
					Peers:                      peers,
					Genesis:                    "genesis.json",
					KeyFile:                    "keys.json",
					RootChainAddress:           "/ip4/127.0.0.1/tcp/26611",
					LedgerReplicationMaxBlocks: 10,
					LedgerReplicationMaxTx:     99,
				}
				return sc
			}(),
		}, {
			args: "money --server-address=srv:1234 --server-max-get-blocks-batch-size=55 --server-max-send-msg-size=65 --server-max-recv-msg-size=66 --server-max-connection-age-ms=77 --server-max-connection-age-grace-ms=88",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.RPCServer = &grpcServerConfiguration{
					Address:                 "srv:1234",
					MaxGetBlocksBatchSize:   55,
					MaxSendMsgSize:          65,
					MaxRecvMsgSize:          66,
					MaxConnectionAgeMs:      77,
					MaxConnectionAgeGraceMs: 88,
				}
				return sc
			}(),
		},
		{
			args: "money --rest-server-address=srv:1111 --rest-server-read-timeout=10s --rest-server-read-header-timeout=11s --rest-server-write-timeout=12s --rest-server-idle-timeout=13s --rest-server-max-header=14 --rest-server-max-body=15",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.RESTServer = &restServerConfiguration{
					Address:           "srv:1111",
					ReadTimeout:       10 * time.Second,
					ReadHeaderTimeout: 11 * time.Second,
					WriteTimeout:      12 * time.Second,
					IdleTimeout:       13 * time.Second,
					MaxHeaderBytes:    14,
					MaxBodyBytes:      15,
				}
				return sc
			}(),
		},
		// Money tx system configuration from ENV
		{
			args: "money",
			envVars: []envVar{
				{"AB_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.RPCServer.Address = "srv:1234"
				return sc
			}(),
		}, {
			args: "money --server-address=srv:666",
			envVars: []envVar{
				{"AB_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.RPCServer.Address = "srv:666"
				return sc
			}(),
		}, {
			args: "money --home=/custom-home-1",
			envVars: []envVar{
				{"AB_HOME", "/custom-home-2"},
				{"AB_CONFIG", "custom-config.props"},
				{"AB_LOGGER_CONFIG", "custom-log-conf.yaml"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    "/custom-home-1",
					CfgFile:    "/custom-home-1/custom-config.props",
					LogCfgFile: "custom-log-conf.yaml",
				}
				return sc
			}(),
		}, {
			args: "money",
			envVars: []envVar{
				{"AB_HOME", "/custom-home"},
				{"AB_CONFIG", "custom-config.props"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    "/custom-home",
					CfgFile:    "/custom-home/custom-config.props",
					LogCfgFile: defaultLoggerConfigFile,
				}
				return sc
			}(),
		}, {
			args: "money",
			envVars: []envVar{
				{"AB_LEDGER_REPLICATION_MAX_BLOCKS", "8"},
				{"AB_LEDGER_REPLICATION_MAX_TRANSACTIONS", "16"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Node.LedgerReplicationMaxBlocks = 8
				sc.Node.LedgerReplicationMaxTx = 16
				return sc
			}(),
		},
	}
	for _, tt := range tests {
		t.Run("money_node_conf|"+tt.args+"|"+envVarsStr(tt.envVars), func(t *testing.T) {
			var actualConfig *moneyNodeConfiguration
			shardRunFunc := func(ctx context.Context, sc *moneyNodeConfiguration) error {
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
			abApp.WithOpts(Opts.NodeRunFunc(shardRunFunc)).Execute(context.Background())
			require.Equal(t, tt.expectedConfig.Node, actualConfig.Node)
			require.Equal(t, tt.expectedConfig, actualConfig)
		})
	}
}

func TestMoneyNodeConfig_ConfigFile(t *testing.T) {
	configFileContents := `
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

	expectedConfig := defaultMoneyNodeConfiguration()
	expectedConfig.Base.CfgFile = f.Name()
	expectedConfig.Base.LogCfgFile = "custom-log-conf.yaml"
	expectedConfig.RPCServer.Address = "srv:1234"
	expectedConfig.RPCServer.MaxRecvMsgSize = 9999

	// Set up runner mock
	var actualConfig *moneyNodeConfiguration
	runFunc := func(ctx context.Context, sc *moneyNodeConfiguration) error {
		actualConfig = sc
		return nil
	}

	abApp := New()
	args := "money --config=" + f.Name()
	abApp.baseCmd.SetArgs(strings.Split(args, " "))
	abApp.WithOpts(Opts.NodeRunFunc(runFunc)).Execute(context.Background())

	require.Equal(t, expectedConfig, actualConfig)
}

func defaultMoneyNodeConfiguration() *moneyNodeConfiguration {
	return &moneyNodeConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: &baseConfiguration{
				HomeDir:    alphabillHomeDir(),
				CfgFile:    filepath.Join(alphabillHomeDir(), defaultConfigFile),
				LogCfgFile: defaultLoggerConfigFile,
			},
		},
		Node: &startNodeConfiguration{
			Address:                    "/ip4/127.0.0.1/tcp/26652",
			RootChainAddress:           "/ip4/127.0.0.1/tcp/26662",
			LedgerReplicationMaxBlocks: 1000,
			LedgerReplicationMaxTx:     10000,
		},
		RPCServer: &grpcServerConfiguration{
			Address:               defaultServerAddr,
			MaxGetBlocksBatchSize: defaultMaxGetBlocksBatchSize,
			MaxRecvMsgSize:        defaultMaxRecvMsgSize,
			MaxSendMsgSize:        defaultMaxSendMsgSize,
		},
		RESTServer: &restServerConfiguration{
			Address:           "",
			ReadTimeout:       0,
			ReadHeaderTimeout: 0,
			WriteTimeout:      0,
			IdleTimeout:       0,
			MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
			MaxBodyBytes:      MaxBodyBytes,
		},
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

func TestRunMoneyNode_Ok(t *testing.T) {
	homeDirMoney := setupTestHomeDir(t, "money")
	keysFileLocation := filepath.Join(homeDirMoney, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDirMoney, nodeGenesisFileName)
	partitionGenesisFileLocation := filepath.Join(homeDirMoney, "partition-genesis.json")
	test.MustRunInTime(t, 5*time.Second, func() {
		port := "9543"
		listenAddr := ":" + port // listen is on all devices, so it would work in CI inside docker too.
		dialAddr := "localhost:" + port

		conf := defaultMoneyNodeConfiguration()
		conf.RPCServer.Address = listenAddr

		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New()
		args := "money-genesis --home " + homeDirMoney + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.addAndExecuteCommand(context.Background())
		require.NoError(t, err)

		pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{})
		require.NoError(t, err)

		// use same keys for signing and communication encryption.
		rootSigner, verifier := testsig.CreateSignerAndVerifier(t)
		rootPubKeyBytes, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pr, err := rootchain.NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn})
		require.NoError(t, err)
		_, partitionGenesisFiles, err := rootchain.NewRootGenesis("test", rootSigner, rootPubKeyBytes, pr)
		require.NoError(t, err)

		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)

		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			cmd = New()
			args = "money --home " + homeDirMoney + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.addAndExecuteCommand(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()

		log.Info("Started money node and dialing...")
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

func makeSuccessfulPayment(t *testing.T, ctx context.Context, txClient alphabill.AlphabillServiceClient) {
	initialBillID := uint256.NewInt(defaultInitialBillId).Bytes32()

	tx := &txsystem.Transaction{
		UnitId:                initialBillID[:],
		TransactionAttributes: new(anypb.Any),
		Timeout:               10,
		OwnerProof:            script.PredicateArgumentEmpty(),
		SystemId:              []byte{0, 0, 0, 0},
	}
	bt := &billtx.TransferOrder{
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

	tx := &txsystem.Transaction{
		UnitId:                wrongBillID[:],
		TransactionAttributes: new(anypb.Any),
		Timeout:               10,
		OwnerProof:            script.PredicateArgumentEmpty(),
		SystemId:              []byte{0},
	}
	bt := &billtx.TransferOrder{
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
