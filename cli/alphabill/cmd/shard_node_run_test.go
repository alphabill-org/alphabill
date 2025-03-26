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
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	testutils "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	test "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/rpc"
)

type envVar [2]string

// TODO: orchestration shardConf custom params
// PartitionParams: map[string]string{
// 	"ownerPredicate": "830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0",
// },

func TestShardNodeRun_EnvAndFlags(t *testing.T) {
	tmpDir := t.TempDir()
	logCfgFilename := filepath.Join(tmpDir, "custom-log-conf.yaml")

	// custom log cfg file with minimal content
	require.NoError(t, os.WriteFile(logCfgFilename, []byte(`log-format: "text"`), 0666))

	tests := []struct {
		name           string
		args           string   // arguments as a space separated string
		envVars        []envVar // Environment variables that will be set before creating command
		expectedConfig *shardNodeRunFlags
	}{
		// Base configuration permutations
		{
			name:           "foo",
			args:           "shard-node run",
			expectedConfig: defaultFlags(),
		}, {
			args: "shard-node run --home=/custom-home",
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.HomeDir = "/custom-home"
				f.CfgFile = filepath.Join("/custom-home", defaultConfigFile)
				return f
			}(),
		}, {
			args: "shard-node run --home=/custom-home --config=custom-config.props",
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.HomeDir = "/custom-home"
				f.CfgFile = "/custom-home/custom-config.props"
				f.LogCfgFile = defaultLoggerConfigFile
				return f
			}(),
		}, {
			args: "shard-node run --config=custom-config.props",
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.HomeDir = alphabillHomeDir()
				f.CfgFile = alphabillHomeDir() + "/custom-config.props"
				f.LogCfgFile = defaultLoggerConfigFile
				return f
			}(),
		},
		// Validator configuration from flags
		{
			args: "shard-node run --rpc-server-address=srv:1234",
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.rpcFlags.Address = "srv:1234"
				return f
			}(),
		},
		{
			args: "shard-node run --rpc-server-address=srv:1111 --rpc-server-read-timeout=10s --rpc-server-read-header-timeout=11s --rpc-server-write-timeout=12s --rpc-server-idle-timeout=13s --rpc-server-max-header=14 --rpc-server-max-body=15 --rpc-server-batch-item-limit=16 --rpc-server-batch-response-size-limit=17",
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.rpcFlags.Address =       "srv:1111"
				f.ReadTimeout =            10 * time.Second
				f.ReadHeaderTimeout =      11 * time.Second
				f.WriteTimeout =           12 * time.Second
				f.IdleTimeout =            13 * time.Second
				f.MaxHeaderBytes =         14
				f.MaxBodyBytes =           15
				f.BatchItemLimit =         16
				f.BatchResponseSizeLimit = 17
				return f
			}(),
		},
		// Money tx system configuration from ENV
		{
			args: "shard-node run",
			envVars: []envVar{
				{"AB_RPC_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.rpcFlags.Address = "srv:1234"
				return f
			}(),
		}, {
			args: "shard-node run --rpc-server-address=srv:666",
			envVars: []envVar{
				{"AB_RPC_SERVER_ADDRESS", "srv:1234"},
			},
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.rpcFlags.Address = "srv:666"
				return f
			}(),
		}, {
			args: "shard-node run --home=/custom-home-1",
			envVars: []envVar{
				{"AB_HOME", "/custom-home-2"},
				{"AB_CONFIG", "custom-config.props"},
				{"AB_LOGGER_CONFIG", logCfgFilename},
			},
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.HomeDir = "/custom-home-1"
				f.CfgFile = "/custom-home-1/custom-config.props"
				f.LogCfgFile = logCfgFilename
				return f
			}(),
		}, {
			args: "shard-node run",
			envVars: []envVar{
				{"AB_HOME", "/custom-home"},
				{"AB_CONFIG", "custom-config.props"},
			},
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.HomeDir = "/custom-home"
				f.CfgFile = "/custom-home/custom-config.props"
				f.LogCfgFile = defaultLoggerConfigFile
				return f
			}(),
		}, {
			args: "shard-node run",
			envVars: []envVar{
				{"AB_LEDGER_REPLICATION_MAX_BLOCKS_FETCH", "4"},
				{"AB_LEDGER_REPLICATION_MAX_BLOCKS", "8"},
				{"AB_LEDGER_REPLICATION_MAX_TRANSACTIONS", "16"},
			},
			expectedConfig: func() *shardNodeRunFlags {
				f := defaultFlags()
				f.LedgerReplicationMaxBlocksFetch = 4
				f.LedgerReplicationMaxBlocks = 8
				f.LedgerReplicationMaxTx = 16
				return f
			}(),
		},
	}
	for _, tt := range tests {
		t.Run("shard_node_run_flags|"+tt.args+"|"+envVarsStr(tt.envVars), func(t *testing.T) {
			var actualFlags *shardNodeRunFlags
			shardNodeRunFn := func(ctx context.Context, flags *shardNodeRunFlags) error {
				actualFlags = flags
				return nil
			}

			// Set environment variables only for single test.
			for _, en := range tt.envVars {
				err := os.Setenv(en[0], en[1])
				require.NoError(t, err)
				defer os.Unsetenv(en[0])
			}

			abApp := New(testobserve.NewFactory(t), Opts.ShardNodeRunFn(shardNodeRunFn))
			abApp.baseCmd.SetArgs(strings.Split(tt.args, " "))
			err := abApp.Execute(context.Background())
			require.NoError(t, err, "executing app command")

			// do not compare observability implementation
			actualFlags.observe = nil
			require.Equal(t, tt.expectedConfig, actualFlags)
		})
	}
}

func TestShardNodeRun_ConfigFile(t *testing.T) {
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

	expectedConfig := defaultFlags()
	expectedConfig.CfgFile = cfgFilename
	expectedConfig.LogCfgFile = logCfgFilename
	expectedConfig.rpcFlags.Address = "srv:1234"

	// Set up runner mock
	var actualConfig *shardNodeRunFlags
	runFunc := func(ctx context.Context, sc *shardNodeRunFlags) error {
		actualConfig = sc
		return nil
	}

	abApp := New(testobserve.NewFactory(t), Opts.ShardNodeRunFn(runFunc))
	args := "shard-node run --config=" + cfgFilename
	abApp.baseCmd.SetArgs(strings.Split(args, " "))
	err := abApp.Execute(context.Background())
	require.NoError(t, err, "executing app command")
	// do not compare observability implementation
	actualConfig.observe = nil
	require.Equal(t, expectedConfig, actualConfig)
}

func defaultFlags() *shardNodeRunFlags {
	flags := &shardNodeRunFlags{
		baseFlags: &baseFlags{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    filepath.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
	}
	flags.KeyConfFile = ""
	flags.ShardConfFile = ""
	flags.TrustBaseFile = ""
	flags.Address = "/ip4/127.0.0.1/tcp/26652"
	flags.LedgerReplicationMaxBlocksFetch = 1000
	flags.LedgerReplicationMaxBlocks = 1000
	flags.LedgerReplicationMaxTx = 10000
	flags.LedgerReplicationTimeoutMs = 1500
	flags.BlockSubscriptionTimeoutMs = 3000
	flags.WithOwnerIndex = true
	flags.WithGetUnits = false
	flags.rpcFlags.Address = ""
	flags.MaxHeaderBytes = http.DefaultMaxHeaderBytes
	flags.MaxBodyBytes = rpc.DefaultMaxBodyBytes
	flags.BatchItemLimit = rpc.DefaultBatchItemLimit
	flags.BatchResponseSizeLimit = rpc.DefaultBatchResponseSizeLimit

	return flags
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

func TestShardNodeRun_Ok(t *testing.T) {
	homeDir := writeShardConf(t, defaultMoneyShardConf)
	trustBaseFile := filepath.Join(homeDir, trustBaseFileName)

	test.MustRunInTime(t, 5*time.Second, func() {
		logF := testobserve.NewFactory(t)
		ctx, ctxCancel := context.WithCancel(context.Background())

		// TODO: some helper function for this cmd execution
		cmd := New(logF)
		cmd.baseCmd.SetArgs([]string{
			"shard-node", "init", "--home", homeDir, "--generate",
		})
		require.NoError(t, cmd.Execute(context.Background()))

		cmd = New(logF)
		cmd.baseCmd.SetArgs([]string{
			"shard-conf", "genesis", "--home", homeDir,
		})
		require.NoError(t, cmd.Execute(context.Background()))

		cmd = New(logF)
		cmd.baseCmd.SetArgs([]string{
			"shard-conf", "generate", "--home", homeDir,
			"--network-id", "3",
			"--partition-id", "1",
			"--partition-type-id", "1",
			"--epoch", "0",
			"--epoch-start", "100",
			"--node-info", filepath.Join(homeDir, nodeInfoFileName),
		})
		require.NoError(t, cmd.Execute(context.Background()))

		_, verifier := testsig.CreateSignerAndVerifier(t)
		trustBase := trustbase.NewTrustBase(t, verifier)
		require.NoError(t, util.WriteJsonFile(trustBaseFile, trustBase))

		rpcServerAddress := fmt.Sprintf("127.0.0.1:%d", net.GetFreeRandomPort(t))

		// start the node in background
		appStoppedWg := sync.WaitGroup{}
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(logF)
			cmd.baseCmd.SetArgs([]string{
				"shard-node", "run",
				"--home", homeDir,
				"--rpc-server-address", rpcServerAddress,
			})
			require.ErrorIs(t, cmd.Execute(ctx), context.Canceled)
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
	initialBillID := moneyPartitionInitialBillID
	attr := &money.TransferAttributes{
		NewOwnerPredicate: templates.AlwaysTrueBytes(),
		TargetValue:       500,
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
		TargetValue:       500,
	}
	attrBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)

	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			Type:           money.TransactionTypeTransfer,
			UnitID:         moneyPartitionInitialBillID[:],
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

// TODO: need to test different txSystems?
func sendTokensTx(t *testing.T, ctx context.Context, rpcClient *ethrpc.Client) {
	attr := &tokens.DefineNonFungibleTokenAttributes{
		Symbol:                   "Test",
		ParentTypeID:             []byte{0},
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
		DataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}
	attrBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)
	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:      types.NetworkLocal,
			PartitionID:    tokens.DefaultPartitionID,
			Type:           tokens.TransactionTypeDefineNFT,
			Attributes:     attrBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 10},
		},
	}
	// require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
	txBytes, err := types.Cbor.Marshal(tx)
	require.NoError(t, err)
	var res hex.Bytes
	err = rpcClient.CallContext(ctx, &res, "state_sendTransaction", hexutil.Encode(txBytes))
	require.NoError(t, err)
	require.NotNil(t, res)

	// failing case
	var res2 hex.Bytes
	tx.PartitionID = 0x01000000 // incorrect partition id
	txBytes, err = types.Cbor.Marshal(tx)
	require.NoError(t, err)
	err = rpcClient.CallContext(ctx, &res2, "state_sendTransaction", hexutil.Encode(txBytes))
	require.ErrorContains(t, err, "invalid transaction partition identifier")
	require.Nil(t, res2)
}

// TODO
// func sendOrchestrationTx(t *testing.T, ctx context.Context, rpcClient *ethrpc.Client) {
// 	// make successful 'addVar' transaction
// 	attr := &orchestration.AddVarAttributes{}
// 	attrBytes, err := types.Cbor.Marshal(attr)
// 	require.NoError(t, err)
// 	varID, err := shardConf.ComposeUnitID(types.ShardID{}, orchestration.VarUnitType, orchid.Random)
// 	require.NoError(t, err)
// 	tx := &types.TransactionOrder{
// 		Version: 1,
// 		Payload: types.Payload{
// 			Type:           orchestration.TransactionTypeAddVAR,
// 			UnitID:         varID,
// 			ClientMetadata: &types.ClientMetadata{Timeout: 10},
// 			PartitionID:    orchestration.DefaultPartitionID,
// 			Attributes:     attrBytes,
// 		},
// 	}
// 	txBytes, err := types.Cbor.Marshal(tx)
// 	require.NoError(t, err)

// 	var res hex.Bytes
// 	err = rpcClient.CallContext(ctx, &res, "state_sendTransaction", hexutil.Encode(txBytes))
// 	require.NoError(t, err)
// 	require.NotNil(t, res)
// }

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
