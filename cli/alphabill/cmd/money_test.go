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

	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	testutils "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	test "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rpc"
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
			args: "money --rpc-server-address=srv:1234",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.rpcServer.Address = "srv:1234"
				return sc
			}(),
		},
		{
			args: "money --rpc-server-address=srv:1111 --rpc-server-read-timeout=10s --rpc-server-read-header-timeout=11s --rpc-server-write-timeout=12s --rpc-server-idle-timeout=13s --rpc-server-max-header=14 --rpc-server-max-body=15 --rpc-server-batch-item-limit=16 --rpc-server-batch-response-size-limit=17",
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.rpcServer = &rpc.ServerConfiguration{
					Address:                "srv:1111",
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
				{"AB_RPC_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.rpcServer.Address = "srv:1234"
				return sc
			}(),
		}, {
			args: "money --rpc-server-address=srv:666",
			envVars: []envVar{
				{"AB_RPC_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.rpcServer.Address = "srv:666"
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
				{"AB_LEDGER_REPLICATION_MAX_BLOCKS_FETCH", "4"},
				{"AB_LEDGER_REPLICATION_MAX_BLOCKS", "8"},
				{"AB_LEDGER_REPLICATION_MAX_TRANSACTIONS", "16"},
			},
			expectedConfig: func() *moneyNodeConfiguration {
				sc := defaultMoneyNodeConfiguration()
				sc.Node.LedgerReplicationMaxBlocksFetch = 4
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
rpc-server-address: "srv:1234"
logger-config: "` + logCfgFilename + `"
`

	// custom log cfg file must exist so store one value there
	require.NoError(t, os.WriteFile(logCfgFilename, []byte(`log-format: "text"`), 0666))

	cfgFilename := filepath.Join(tmpDir, "custom-conf.yaml")
	require.NoError(t, os.WriteFile(cfgFilename, []byte(configFileContents), 0666))

	expectedConfig := defaultMoneyNodeConfiguration()
	expectedConfig.Base.CfgFile = cfgFilename
	expectedConfig.Base.LogCfgFile = logCfgFilename
	expectedConfig.rpcServer.Address = "srv:1234"

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
			Address:                         "/ip4/127.0.0.1/tcp/26652",
			LedgerReplicationMaxBlocksFetch: 1000,
			LedgerReplicationMaxBlocks:      1000,
			LedgerReplicationMaxTx:          10000,
			LedgerReplicationTimeoutMs:      1500,
			BlockSubscriptionTimeoutMs:      3000,
			WithOwnerIndex:                  true,
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
	homeDirMoney := t.TempDir()
	keysFileLocation := filepath.Join(homeDirMoney, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDirMoney, moneyGenesisFileName)
	nodeGenesisStateFileLocation := filepath.Join(homeDirMoney, moneyGenesisStateFileName)
	partitionGenesisFileLocation := filepath.Join(homeDirMoney, "partition-genesis.json")
	trustBaseFileLocation := filepath.Join(homeDirMoney, rootTrustBaseFileName)
	pdrFilename, err := createPDRFile(homeDirMoney, defaultMoneyPDR)
	require.NoError(t, err)
	test.MustRunInTime(t, 5*time.Second, func() {
		logF := testobserve.NewFactory(t)
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New(logF)
		args := "money-genesis --home " + homeDirMoney +
			" --partition-description " + pdrFilename +
			" -o " + nodeGenesisFileLocation +
			" --output-state " + nodeGenesisStateFileLocation +
			" -g -k " + keysFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.NoError(t, err)

		pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{Version: 1})
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
		rootGenesis, partitionGenesisFiles, err := rootgenesis.NewRootGenesis(rootID.String(), rootSigner, rootPubKeyBytes, pr)
		require.NoError(t, err)
		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)
		trustBase, err := rootGenesis.GenerateTrustBase()
		require.NoError(t, err)
		err = util.WriteJsonFile(trustBaseFileLocation, trustBase)
		require.NoError(t, err)
		rpcServerAddress := fmt.Sprintf("127.0.0.1:%d", net.GetFreeRandomPort(t))

		// start the node in background
		appStoppedWg := sync.WaitGroup{}
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(logF)
			args = "money --home " + homeDirMoney +
				" -g " + partitionGenesisFileLocation +
				" -s " + nodeGenesisStateFileLocation +
				" -t " + trustBaseFileLocation +
				" -k " + keysFileLocation +
				" --rpc-server-address " + rpcServerAddress
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.Execute(ctx)
			require.ErrorIs(t, err, context.Canceled)
		}()

		t.Log("Started money node and dialing...")

		// create rpc client
		rpcClient, err := ethrpc.DialContext(ctx, buildRpcUrl(rpcServerAddress))
		require.NoError(t, err)
		defer rpcClient.Close()

		// wait for rpc server to start
		require.Eventually(t, func() bool {
			var res *rpc.NodeInfoResponse
			err := rpcClient.CallContext(ctx, &res, "admin_getNodeInfo")
			return err == nil && res != nil
		}, testutils.WaitDuration, testutils.WaitTick)

		// Test cases
		makeSuccessfulPayment(t, ctx, rpcClient)
		makeFailingPayment(t, ctx, rpcClient)

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func makeSuccessfulPayment(t *testing.T, ctx context.Context, rpcClient *ethrpc.Client) {
	initialBillID := defaultInitialBillID
	attr := &money.TransferAttributes{
		NewOwnerPredicate: templates.AlwaysTrueBytes(),
		TargetValue:       defaultInitialBillValue,
	}
	attrBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)

	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			Type:           money.TransactionTypeTransfer,
			UnitID:         initialBillID[:],
			ClientMetadata: &types.ClientMetadata{Timeout: 10},
			PartitionID:    money.DefaultPartitionID,
			Attributes:     attrBytes,
		},
	}
	require.NoError(t, tx.SetAuthProof(money.TransferAuthProof{}))
	txBytes, err := types.Cbor.Marshal(tx)
	require.NoError(t, err)

	var res hex.Bytes
	err = rpcClient.CallContext(ctx, &res, "state_sendTransaction", hexutil.Encode(txBytes))
	require.NoError(t, err)
	require.NotNil(t, res)
}

func makeFailingPayment(t *testing.T, ctx context.Context, rpcClient *ethrpc.Client) {
	attr := &money.TransferAttributes{
		NewOwnerPredicate: templates.AlwaysTrueBytes(),
		TargetValue:       defaultInitialBillValue,
	}
	attrBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)

	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			Type:           money.TransactionTypeTransfer,
			UnitID:         defaultInitialBillID[:],
			ClientMetadata: &types.ClientMetadata{Timeout: 10},
			PartitionID:    0, // invalid partition id
			Attributes:     attrBytes,
		},
	}
	require.NoError(t, tx.SetAuthProof(money.TransferAuthProof{}))
	txBytes, err := types.Cbor.Marshal(tx)
	require.NoError(t, err)

	var res hex.Bytes
	err = rpcClient.CallContext(ctx, &res, "state_sendTransaction", hexutil.Encode(txBytes))
	require.ErrorContains(t, err, "failed to submit transaction to the network: expected 00000001, got 00000000: invalid transaction partition identifier")
	require.Nil(t, res, "Failing payment should not return response")
}

func buildRpcUrl(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	url = strings.TrimSuffix(url, "/")
	if !strings.HasSuffix(url, "/rpc") {
		url = url + "/rpc"
	}
	return url
}
