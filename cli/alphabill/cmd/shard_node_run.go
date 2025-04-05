package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	orchestrationsdk "github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	tokenssdk "github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/orchestration"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
)

const (
	shardStoreFileName = "shard.db"
	blockStoreFileName = "blocks.db"
	proofStoreFileName = "proof.db"
)

type shardNodeRunFlags struct {
	*baseFlags
	keyConfFlags
	shardConfFlags
	trustBaseFlags
	p2pFlags
	rpcFlags

	StateFile      string
	BlockStoreFile string
	ProofStoreFile string
	ShardStoreFile string

	WithOwnerIndex bool
	WithGetUnits   bool

	LedgerReplicationMaxBlocksFetch uint64
	LedgerReplicationMaxBlocks      uint64
	LedgerReplicationMaxTx          uint32
	LedgerReplicationTimeoutMs      uint64
	BlockSubscriptionTimeoutMs      uint64
}

func shardNodeRunCmd(baseFlags *baseFlags, shardNodeRunFn nodeRunnable) *cobra.Command {
	flags := &shardNodeRunFlags{baseFlags: baseFlags}
	var cmd = &cobra.Command{
		Use:   "run",
		Short: "Starts a shard node",
		Long:  `Starts a shard node for the shard described in shard configuration`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shardNodeRunFn != nil {
				return shardNodeRunFn(cmd.Context(), flags)
			}
			return shardNodeRun(cmd.Context(), flags)
		},
	}

	flags.addKeyConfFlags(cmd, false)
	flags.addTrustBaseFlags(cmd)
	flags.addShardConfFlags(cmd)
	flags.addP2PFlags(cmd)
	flags.addRPCFlags(cmd)

	cmd.Flags().StringVarP(&flags.StateFile, "state", "", "",
		fmt.Sprintf("path to the state file (default %s)", filepath.Join("$AB_HOME", stateFileName)))
	cmd.Flags().StringVarP(&flags.BlockStoreFile, "block-db", "", "",
		fmt.Sprintf("path to the block datatabase (default %s)", filepath.Join("$AB_HOME", blockStoreFileName)))
	cmd.Flags().StringVarP(&flags.ShardStoreFile, "shard-db", "", "",
		fmt.Sprintf("path to the shard configuration datatabase (default %s)", filepath.Join("$AB_HOME", shardStoreFileName)))
	cmd.Flags().StringVarP(&flags.ProofStoreFile, "proof-db", "", "",
		fmt.Sprintf("path to the proof datatabase (default %s)", filepath.Join("$AB_HOME", proofStoreFileName)))

	cmd.Flags().BoolVar(&flags.WithOwnerIndex, "with-owner-index", true, "enable/disable owner indexer")
	cmd.Flags().BoolVar(&flags.WithGetUnits, "with-get-units", false, "enable/disable state_getUnits RPC endpoint")

	cmd.Flags().Uint64Var(&flags.LedgerReplicationMaxBlocksFetch, "ledger-replication-max-blocks-fetch", 1000,
		"maximum number of blocks to query in a single replication request")
	cmd.Flags().Uint64Var(&flags.LedgerReplicationMaxBlocks, "ledger-replication-max-blocks", 1000,
		"maximum number of blocks to return in a single replication response")
	cmd.Flags().Uint32Var(&flags.LedgerReplicationMaxTx, "ledger-replication-max-transactions", 10000,
		"maximum number of transactions to return in a single replication response")
	cmd.Flags().Uint64Var(&flags.LedgerReplicationTimeoutMs, "ledger-replication-timeout", 1500,
		"time since last received replication response when to trigger another request (in ms)")
	cmd.Flags().Uint64Var(&flags.BlockSubscriptionTimeoutMs, "block-subscription-timeout", 3000,
		"time since last received block when when to trigger recovery (in ms) for non-validating nodes")

	return cmd
}

func shardNodeRun(ctx context.Context, flags *shardNodeRunFlags) error {
	node, nodeConf, err := createNode(ctx, flags)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	obs := nodeConf.Observability()
	log := obs.Logger()
	partitionType := partitionTypeIDToString(node.PartitionTypeID())

	log.InfoContext(ctx, fmt.Sprintf("starting %s node: BuildInfo=%s", partitionType, debug.ReadBuildInfo()))
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return node.Run(ctx) })

	g.Go(func() error {
		if flags.rpcFlags.IsAddressEmpty() {
			return nil // return nil in this case in order not to kill the group!
		}
		routers := []rpc.Registrar{
			rpc.MetricsEndpoints(obs.PrometheusRegisterer()),
			rpc.NodeEndpoints(node, obs),
		}
		if flags.rpcFlags.Router != nil {
			routers = append(routers, flags.rpcFlags.Router)
		}
		flags.rpcFlags.APIs = []rpc.API{
			{
				Namespace: "state",
				Service: rpc.NewStateAPI(node, obs,
					rpc.WithOwnerIndex(nodeConf.OwnerIndexer()),
					rpc.WithGetUnits(flags.WithGetUnits),
					rpc.WithShardConf(nodeConf.ShardConf()),
					rpc.WithRateLimit(flags.StateRpcRateLimit)),
			},
			{
				Namespace: "admin",
				Service:   rpc.NewAdminAPI(node, node.Peer(), obs),
			},
		}

		rpcServer, err := rpc.NewHTTPServer(&flags.rpcFlags.ServerConfiguration, obs, routers...)
		if err != nil {
			return err
		}

		errch := make(chan error, 1)
		go func() {
			log.InfoContext(ctx, fmt.Sprintf("%s RPC server starting on %s", partitionType, rpcServer.Addr))
			if err := rpcServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errch <- err
				return
			}
			errch <- nil
		}()

		select {
		case <-ctx.Done():
			if err := rpcServer.Close(); err != nil {
				log.WarnContext(ctx, partitionType+" RPC server close error", logger.Error(err))
			}
			exitErr := <-errch
			if exitErr != nil {
				log.WarnContext(ctx, partitionType+" RPC server exited with error", logger.Error(err))
			} else {
				log.InfoContext(ctx, partitionType+" RPC server exited")
			}
			return ctx.Err()
		case err := <-errch:
			return err
		}
	})

	return g.Wait()
}

func createNode(ctx context.Context, flags *shardNodeRunFlags) (*partition.Node, *partition.NodeConf, error) {
	keyConf, err := flags.loadKeyConf(flags.baseFlags, false)
	if err != nil {
		return nil, nil, err
	}
	shardConf, err := flags.loadShardConf()
	if err != nil {
		return nil, nil, err
	}
	trustBase, err := flags.loadTrustBase()
	if err != nil {
		return nil, nil, err
	}

	shardStore, err := flags.initStore(flags.ShardStoreFile, shardStoreFileName)
	if err != nil {
		return nil, nil, err
	}
	blockStore, err := flags.initStore(flags.BlockStoreFile, blockStoreFileName)
	if err != nil {
		return nil, nil, err
	}
	proofStore, err := flags.initStore(flags.ProofStoreFile, proofStoreFileName)
	if err != nil {
		return nil, nil, err
	}

	nodeID, err := keyConf.NodeID()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to calculate nodeID: %w", err)
	}
	log := flags.observe.Logger().With(
		logger.NodeID(nodeID),
		logger.Shard(shardConf.PartitionID, shardConf.ShardID))
	obs := observability.WithLogger(flags.observe, log)

	var ownerIndexer *partition.OwnerIndexer
	if flags.WithOwnerIndex {
		ownerIndexer = partition.NewOwnerIndexer(log)
	}

	nodeConf, err := partition.NewNodeConf(
		keyConf,
		shardConf,
		trustBase,
		obs,
		partition.WithAddress(flags.p2pFlags.Address),
		partition.WithAnnounceAddresses(flags.AnnounceAddresses),
		partition.WithBootstrapAddresses(flags.BootstrapAddresses),
		partition.WithBlockStore(blockStore),
		partition.WithShardStore(shardStore),
		partition.WithReplicationParams(
			flags.LedgerReplicationMaxBlocksFetch,
			flags.LedgerReplicationMaxBlocks,
			flags.LedgerReplicationMaxTx,
			time.Duration(flags.LedgerReplicationTimeoutMs)*time.Millisecond),
		partition.WithProofIndex(proofStore, 20),
		partition.WithOwnerIndex(ownerIndexer),
		partition.WithBlockSubscriptionTimeout(time.Duration(flags.BlockSubscriptionTimeoutMs) * time.Millisecond),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create node configuration: %w", err)
	}

	txSystem, err := createTxSystem(flags, nodeConf)
	if err != nil {
		return nil, nil, err
	}
	node, err := partition.NewNode(ctx, txSystem, nodeConf)
	if err != nil {
		return nil, nil, err
	}
	return node, nodeConf, nil
}

func (f *shardNodeRunFlags) loadShardConf() (ret *types.PartitionDescriptionRecord, err error) {
	return ret, f.loadConf(f.ShardConfFile, shardConfFileName, &ret)
}

func (f *shardNodeRunFlags) loadTrustBase() (ret *types.RootTrustBaseV1, err error) {
	return ret, f.loadConf(f.TrustBaseFile, trustBaseFileName, &ret)
}

func partitionTypeIDToString(partitionTypeID types.PartitionTypeID) string {
	switch partitionTypeID {
	case moneysdk.PartitionTypeID:
		return money.PartitionType
	case tokenssdk.PartitionTypeID:
		return tokens.PartitionType
	case evmsdk.PartitionTypeID:
		return evm.PartitionType
	case orchestrationsdk.PartitionTypeID:
		return orchestration.PartitionType
	default:
		return "unknown"
	}
}
