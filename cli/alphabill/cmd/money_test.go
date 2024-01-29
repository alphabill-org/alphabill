package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	test "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/predicates/templates"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type envVar [2]string

func TestMoneyNodeConfig_EnvAndFlags(t *testing.T) {
	tmpDir := t.TempDir()
	logCfgFilename := filepath.Join(tmpDir, "custom-log-conf.yaml")

	// custom log cfg file with minimal content
	require.NoError(t, os.WriteFile(logCfgFilename, []byte(`log-format: "text"`), 0666))

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
			args: "money --server-address=srv:1234 --server-max-get-blocks-batch-size=55 --server-max-send-msg-size=65 --server-max-recv-msg-size=66 --server-max-connection-age-ms=77 --server-max-connection-age-grace-ms=88",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.grpcServer = &grpcServerConfiguration{
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
			args: "money --rpc-server-address=srv:1111 --rpc-server-read-timeout=10s --rpc-server-read-header-timeout=11s --rpc-server-write-timeout=12s --rpc-server-idle-timeout=13s --rpc-server-max-header=14 --rpc-server-max-body=15 --rpc-server-batch-item-limit=16 --rpc-server-batch-response-size-limit=17",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.rpcServer = &rpc.ServerConfiguration{
					Address:           "srv:1111",
					ReadTimeout:            10 * time.Second,
					ReadHeaderTimeout:      11 * time.Second,
					WriteTimeout:           12 * time.Second,
					IdleTimeout:            13 * time.Second,
					MaxHeaderBytes:         14,
					MaxBodyBytes:           15,
					BatchItemLimit:         16,
					BatchResponseSizeLimit: 17,
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
				sc.grpcServer.Address = "srv:1234"
				return sc
			}(),
		}, {
			args: "money --server-address=srv:666",
			envVars: []envVar{
				{"AB_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.grpcServer.Address = "srv:666"
				return sc
			}(),
		}, {
			args: "money --home=/custom-home-1",
			envVars: []envVar{
				{"AB_HOME", "/custom-home-2"},
				{"AB_CONFIG", "custom-config.props"},
				{"AB_LOGGER_CONFIG", logCfgFilename},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Base = &baseConfiguration{
					HomeDir:    "/custom-home-1",
					CfgFile:    "/custom-home-1/custom-config.props",
					LogCfgFile: logCfgFilename,
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

			abApp := New(testobserve.NewFactory(t), Opts.NodeRunFunc(shardRunFunc))
			abApp.baseCmd.SetArgs(strings.Split(tt.args, " "))
			err := abApp.Execute(context.Background())
			require.NoError(t, err, "executing app command")
			require.Equal(t, tt.expectedConfig.Node, actualConfig.Node)
			// do not compare observability implementation
			actualConfig.Base.observe = nil
			require.Equal(t, tt.expectedConfig, actualConfig)
		})
	}
}

func TestMoneyNodeConfig_ConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	logCfgFilename := filepath.Join(tmpDir, "custom-log-conf.yaml")

	configFileContents := `
server-address: "srv:1234"
server-max-recv-msg-size: 9999
logger-config: "` + logCfgFilename + `"
`

	// custom log cfg file must exist so store one value there
	require.NoError(t, os.WriteFile(logCfgFilename, []byte(`log-format: "text"`), 0666))

	cfgFilename := filepath.Join(tmpDir, "custom-conf.yaml")
	require.NoError(t, os.WriteFile(cfgFilename, []byte(configFileContents), 0666))

	expectedConfig := defaultMoneyNodeConfiguration()
	expectedConfig.Base.CfgFile = cfgFilename
	expectedConfig.Base.LogCfgFile = logCfgFilename
	expectedConfig.grpcServer.Address = "srv:1234"
	expectedConfig.grpcServer.MaxRecvMsgSize = 9999

	// Set up runner mock
	var actualConfig *moneyNodeConfiguration
	runFunc := func(ctx context.Context, sc *moneyNodeConfiguration) error {
		actualConfig = sc
		return nil
	}

	abApp := New(testobserve.NewFactory(t), Opts.NodeRunFunc(runFunc))
	args := "money --config=" + cfgFilename
	abApp.baseCmd.SetArgs(strings.Split(args, " "))
	err := abApp.Execute(context.Background())
	require.NoError(t, err, "executing app command")
	// do not compare observability implementation
	actualConfig.Base.observe = nil
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
			LedgerReplicationMaxBlocks: 1000,
			LedgerReplicationMaxTx:     10000,
			WithOwnerIndex:             true,
		},
		grpcServer: &grpcServerConfiguration{
			Address:               defaultServerAddr,
			MaxGetBlocksBatchSize: defaultMaxGetBlocksBatchSize,
			MaxRecvMsgSize:        defaultMaxRecvMsgSize,
			MaxSendMsgSize:        defaultMaxSendMsgSize,
		},
		rpcServer: &rpc.ServerConfiguration{
			Address:                "",
			ReadTimeout:            0,
			ReadHeaderTimeout:      0,
			WriteTimeout:           0,
			IdleTimeout:            0,
			MaxHeaderBytes:         http.DefaultMaxHeaderBytes,
			MaxBodyBytes:           rpc.DefaultMaxBodyBytes,
			BatchItemLimit:         rpc.DefaultBatchItemLimit,
			BatchResponseSizeLimit: rpc.DefaultBatchResponseSizeLimit,
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
	nodeGenesisFileLocation := filepath.Join(homeDirMoney, moneyGenesisFileName)
	nodeGenesisStateFileLocation := filepath.Join(homeDirMoney, moneyGenesisStateFileName)
	partitionGenesisFileLocation := filepath.Join(homeDirMoney, "partition-genesis.json")
	test.MustRunInTime(t, 5*time.Second, func() {
		moneyNodeAddr := fmt.Sprintf("127.0.0.1:%d", net.GetFreeRandomPort(t))
		logF := testobserve.NewFactory(t)

		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New(logF)
		args := "money-genesis --home " + homeDirMoney +
			" -o " + nodeGenesisFileLocation +
			" --output-state " + nodeGenesisStateFileLocation +
			" -g -k " + keysFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.NoError(t, err)

		pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{})
		require.NoError(t, err)

		// use same keys for signing and communication encryption.
		rootSigner, verifier := testsig.CreateSignerAndVerifier(t)
		rootPubKeyBytes, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn})
		require.NoError(t, err)
		rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(rootPubKeyBytes)
		require.NoError(t, err)
		rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
		require.NoError(t, err)
		bootNodeStr := fmt.Sprintf("%s@/ip4/127.0.0.1/tcp/26662", rootID.String())
		_, partitionGenesisFiles, err := rootgenesis.NewRootGenesis(rootID.String(), rootSigner, rootPubKeyBytes, pr)
		require.NoError(t, err)

		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)

		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			cmd = New(logF)
			args = "money --home " + homeDirMoney +
				" -g " + partitionGenesisFileLocation +
				" -s " + nodeGenesisStateFileLocation +
				" -k " + keysFileLocation +
				" --bootnodes=" + bootNodeStr +
				" --server-address " + moneyNodeAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.Execute(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()

		t.Log("Started money node and dialing...")
		// Create the gRPC client
		conn, err := grpc.DialContext(ctx, moneyNodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
	initialBillID := defaultInitialBillID
	attr := &money.TransferAttributes{
		NewBearer:   templates.AlwaysTrueBytes(),
		TargetValue: defaultInitialBillValue,
	}
	attrBytes, err := cbor.Marshal(attr)
	require.NoError(t, err)
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           money.PayloadTypeTransfer,
			UnitID:         initialBillID[:],
			ClientMetadata: &types.ClientMetadata{Timeout: 10},
			SystemID:       money.DefaultSystemIdentifier,
			Attributes:     attrBytes,
		},
		OwnerProof: nil,
	}
	txBytes, err := cbor.Marshal(tx)
	require.NoError(t, err)
	protoTx := &alphabill.Transaction{Order: txBytes}
	_, err = txClient.ProcessTransaction(ctx, protoTx, grpc.WaitForReady(true))
	require.NoError(t, err)
}

func makeFailingPayment(t *testing.T, ctx context.Context, txClient alphabill.AlphabillServiceClient) {
	wrongBillID := money.NewBillID(nil, []byte{6})
	attr := &money.TransferAttributes{
		NewBearer:   templates.AlwaysTrueBytes(),
		TargetValue: defaultInitialBillValue,
	}
	attrBytes, err := cbor.Marshal(attr)
	require.NoError(t, err)
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           money.PayloadTypeTransfer,
			UnitID:         wrongBillID,
			ClientMetadata: &types.ClientMetadata{Timeout: 10},
			SystemID:       0,
			Attributes:     attrBytes,
		},
		OwnerProof: nil,
	}
	txBytes, err := cbor.Marshal(tx)
	require.NoError(t, err)
	protoTx := &alphabill.Transaction{Order: txBytes}
	response, err := txClient.ProcessTransaction(ctx, protoTx, grpc.WaitForReady(true))
	require.Error(t, err)
	require.Nil(t, response, "Failing payment should not return response")
}
