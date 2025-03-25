package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

const (
	cmdFlagState         = "state"
	cmdFlagTrustBaseFile = "trust-base-file"
)

type baseNodeConfiguration struct {
	Base *baseConfiguration
}

type startNodeConfiguration struct {
	Address                         string
	AnnounceAddrs                   []string
	Genesis                         string
	StateFile                       string
	TrustBaseFile                   string
	KeyFile                         string
	DbFile                          string
	ShardConfigurationDbFile        string
	TxIndexerDBFile                 string
	WithOwnerIndex                  bool
	WithGetUnits                    bool
	LedgerReplicationMaxBlocksFetch uint64
	LedgerReplicationMaxBlocks      uint64
	LedgerReplicationMaxTx          uint32
	LedgerReplicationTimeoutMs      uint64
	BlockSubscriptionTimeoutMs      uint64
	BootStrapAddresses              []string // boot strap addresses (libp2p multiaddress format)
	GetUnitsByOwnerIDRateLimit      int
}

func run(
	ctx context.Context,
	node *partition.Node,
	rpcServerConf *rpc.ServerConfiguration,
	ownerIndexer *partition.OwnerIndexer,
	withGetUnits bool,
	pdr *types.PartitionDescriptionRecord,
	obs Observability,
	getUnitsByOwnerIDRateLimit int,
) error {
	log := obs.Logger()
	name := partitionTypeIDToName(node.PartitionTypeID())
	log.InfoContext(ctx, fmt.Sprintf("starting %s node: BuildInfo=%s", name, debug.ReadBuildInfo()))
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return node.Run(ctx) })

	g.Go(func() error {
		if rpcServerConf.IsAddressEmpty() {
			return nil // return nil in this case in order not to kill the group!
		}
		routers := []rpc.Registrar{
			rpc.MetricsEndpoints(obs.PrometheusRegisterer()),
			rpc.NodeEndpoints(node, obs),
		}
		if rpcServerConf.Router != nil {
			routers = append(routers, rpcServerConf.Router)
		}
		rpcServerConf.APIs = []rpc.API{
			{
				Namespace: "state",
				Service: rpc.NewStateAPI(
					node,
					obs,
					rpc.WithOwnerIndex(ownerIndexer),
					rpc.WithGetUnits(withGetUnits),
					rpc.WithPDR(pdr),
					rpc.WithGetUnitsByOwnerIDRateLimit(getUnitsByOwnerIDRateLimit),
				),
			},
			{
				Namespace: "admin",
				Service:   rpc.NewAdminAPI(node, node.Peer(), obs),
			},
		}

		rpcServer, err := rpc.NewHTTPServer(rpcServerConf, obs, routers...)
		if err != nil {
			return err
		}

		errch := make(chan error, 1)
		go func() {
			log.InfoContext(ctx, fmt.Sprintf("%s RPC server starting on %s", name, rpcServer.Addr))
			if err := rpcServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errch <- err
				return
			}
			errch <- nil
		}()

		select {
		case <-ctx.Done():
			if err := rpcServer.Close(); err != nil {
				log.WarnContext(ctx, name+" RPC server close error", logger.Error(err))
			}
			exitErr := <-errch
			if exitErr != nil {
				log.WarnContext(ctx, name+" RPC server exited with error", logger.Error(err))
			} else {
				log.InfoContext(ctx, name+" RPC server exited")
			}
			return ctx.Err()
		case err := <-errch:
			return err
		}
	})

	return g.Wait()
}

func loadPeerConfiguration(keys *Keys, cfg *startNodeConfiguration) (*network.PeerConfiguration, error) {
	pair, err := keys.getAuthKeyPair()
	if err != nil {
		return nil, err
	}
	bootNodes, err := getBootStrapNodes(cfg.BootStrapAddresses)
	if err != nil {
		return nil, fmt.Errorf("boot nodes parameter error: %w", err)
	}
	return network.NewPeerConfiguration(cfg.Address, cfg.AnnounceAddrs, pair, bootNodes)
}

func createNode(ctx context.Context,
	txs txsystem.TransactionSystem,
	cfg *startNodeConfiguration,
	keys *Keys,
	blockStore keyvaluedb.KeyValueDB,
	proofStore keyvaluedb.KeyValueDB,
	ownerIndexer *partition.OwnerIndexer,
	trustBase types.RootTrustBase,
	obs Observability,
) (*partition.Node, error) {
	pg, err := loadPartitionGenesis(cfg.Genesis)
	if err != nil {
		return nil, err
	}
	// Load network configuration. In testnet, we assume that all validators know the address of all other validators.
	peerConf, err := loadPeerConfiguration(keys, cfg)
	if err != nil {
		return nil, err
	}
	if blockStore == nil {
		blockStore, err = initStore(cfg.DbFile)
		if err != nil {
			return nil, err
		}
	}
	shardStore, err := initStore(cfg.ShardConfigurationDbFile)
	if err != nil {
		return nil, err
	}

	options := []partition.NodeOption{
		partition.WithBlockStore(blockStore),
		partition.WithShardStore(shardStore),
		partition.WithReplicationParams(cfg.LedgerReplicationMaxBlocksFetch, cfg.LedgerReplicationMaxBlocks, cfg.LedgerReplicationMaxTx, time.Duration(cfg.LedgerReplicationTimeoutMs)*time.Millisecond),
		partition.WithProofIndex(proofStore, 20), // TODO history size!
		partition.WithOwnerIndex(ownerIndexer),
		partition.WithBlockSubscriptionTimeout(time.Duration(cfg.BlockSubscriptionTimeoutMs) * time.Millisecond),
	}

	node, err := partition.NewNode(
		ctx,
		peerConf,
		keys.Signer,
		txs,
		pg,
		trustBase,
		nil,
		obs,
		options...,
	)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func initStore(dbFile string) (keyvaluedb.KeyValueDB, error) {
	if dbFile != "" {
		return boltdb.New(dbFile)
	}
	return memorydb.New()
}

func loadPartitionGenesis(genesisPath string) (*genesis.PartitionGenesis, error) {
	pg, err := util.ReadJsonFile(genesisPath, &genesis.PartitionGenesis{})
	if err != nil {
		return nil, err
	}
	return pg, nil
}

func loadStateFile(stateFilePath string, unitDataConstructor state.UnitDataConstructor) (*state.State, error) {
	if !util.FileExists(stateFilePath) {
		return nil, fmt.Errorf("state file '%s' not found", stateFilePath)
	}

	stateFile, err := os.Open(filepath.Clean(stateFilePath))
	if err != nil {
		return nil, err
	}
	defer stateFile.Close()

	s, err := state.NewRecoveredState(stateFile, unitDataConstructor)
	if err != nil {
		return nil, fmt.Errorf("failed to build state tree from state file: %w", err)
	}
	return s, nil
}

func addCommonNodeConfigurationFlags(nodeCmd *cobra.Command, config *startNodeConfiguration, partitionSuffix string) {
	nodeCmd.Flags().StringVarP(&config.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "node address in libp2p multiaddress-format")
	nodeCmd.Flags().StringSliceVarP(&config.AnnounceAddrs, "announce-addresses", "b", nil, "node public ip addresses in libp2p multiaddress-format, if specified overwrites any and all default listen addresses")
	nodeCmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", fmt.Sprintf("path to the key file (default: $AB_HOME/%s/keys.json)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.Genesis, "genesis", "g", "", fmt.Sprintf("path to the partition genesis file : $AB_HOME/%s/partition-genesis.json)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.StateFile, cmdFlagState, "s", "", fmt.Sprintf("path to the state file : $AB_HOME/%s/node-genesis-state.cbor)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.TrustBaseFile, cmdFlagTrustBaseFile, "t", "", "path to the root trust base file : $AB_HOME/root-trust-base.json)")
	nodeCmd.Flags().StringSliceVar(&config.BootStrapAddresses, rootBootStrapNodesCmdFlag, nil, "list of bootstrap root node addresses libp2p multiaddress format")
	nodeCmd.Flags().StringVarP(&config.DbFile, "db", "f", "", "path to the block database file; in-memory database is used if not set")
	nodeCmd.Flags().StringVarP(&config.ShardConfigurationDbFile, "shard-db", "", "", "path to the shard configuration database file; in-memory database is used if not set")
	nodeCmd.Flags().StringVarP(&config.TxIndexerDBFile, "tx-db", "", "", "path to the transaction indexer database file; in-memory database is used if not set")
	nodeCmd.Flags().BoolVar(&config.WithOwnerIndex, "with-owner-index", true, "enable/disable owner indexer")
	nodeCmd.Flags().BoolVar(&config.WithGetUnits, "with-get-units", false, "enable/disable state_getUnits RPC endpoint")
	nodeCmd.Flags().Uint64Var(&config.LedgerReplicationMaxBlocksFetch, "ledger-replication-max-blocks-fetch", 1000, "maximum number of blocks to query in a single replication request")
	nodeCmd.Flags().Uint64Var(&config.LedgerReplicationMaxBlocks, "ledger-replication-max-blocks", 1000, "maximum number of blocks to return in a single replication response")
	nodeCmd.Flags().Uint32Var(&config.LedgerReplicationMaxTx, "ledger-replication-max-transactions", 10000, "maximum number of transactions to return in a single replication response")
	nodeCmd.Flags().Uint64Var(&config.LedgerReplicationTimeoutMs, "ledger-replication-timeout", 1500, "time since last received replication response when to trigger another request (in ms)")
	nodeCmd.Flags().Uint64Var(&config.BlockSubscriptionTimeoutMs, "block-subscription-timeout", 3000, "time since last received block when when to trigger recovery (in ms) for non-validating nodes")
	nodeCmd.Flags().IntVar(&config.GetUnitsByOwnerIDRateLimit, "get-units-by-owner-id-rate-limit", 100, "number state_GetUnitsByOwnerID requests allowed in a second")
}

func addRPCServerConfigurationFlags(cmd *cobra.Command, c *rpc.ServerConfiguration) {
	cmd.Flags().StringVar(&c.Address, "rpc-server-address", "",
		"Specifies the TCP address for the RPC server to listen on, in the form \"host:port\". RPC server isn't initialised if Address is empty. (default \"\")")

	cmd.Flags().DurationVar(&c.ReadTimeout, "rpc-server-read-timeout", 0,
		"The maximum duration for reading the entire request, including the body. A zero or negative value means there will be no timeout. (default 0)")
	cmd.Flags().DurationVar(&c.ReadHeaderTimeout, "rpc-server-read-header-timeout", 0,
		"The amount of time allowed to read request headers. If rpc-server-read-header-timeout is zero, the value of rpc-server-read-timeout is used. If both are zero, there is no timeout. (default 0)")
	cmd.Flags().DurationVar(&c.WriteTimeout, "rpc-server-write-timeout", 0,
		"The maximum duration before timing out writes of the response. A zero or negative value means there will be no timeout. (default 0)")
	cmd.Flags().DurationVar(&c.IdleTimeout, "rpc-server-idle-timeout", 0,
		"The maximum amount of time to wait for the next request when keep-alives are enabled. If rpc-server-idle-timeout is zero, the value of rpc-server-read-timeout is used. If both are zero, there is no timeout. (default 0)")
	cmd.Flags().IntVar(&c.MaxHeaderBytes, "rpc-server-max-header", http.DefaultMaxHeaderBytes,
		"Controls the maximum number of bytes the server will read parsing the request header's keys and values, including the request line.")
	cmd.Flags().Int64Var(&c.MaxBodyBytes, "rpc-server-max-body", rpc.DefaultMaxBodyBytes,
		"Controls the maximum number of bytes the server will read parsing the request body.")
	cmd.Flags().IntVar(&c.BatchItemLimit, "rpc-server-batch-item-limit", rpc.DefaultBatchItemLimit,
		"The maximum number of requests in a batch.")
	cmd.Flags().IntVar(&c.BatchResponseSizeLimit, "rpc-server-batch-response-size-limit", rpc.DefaultBatchResponseSizeLimit,
		"The maximum number of response bytes across all requests in a batch.")

	hideFlags(cmd,
		"rpc-server-read-timeout",
		"rpc-server-read-header-timeout",
		"rpc-server-write-timeout",
		"rpc-server-idle-timeout",
		"rpc-server-max-header",
		"rpc-server-max-body",
		"rpc-server-batch-item-limit",
		"rpc-server-batch-response-size-limit",
	)
}

func hideFlags(cmd *cobra.Command, flags ...string) {
	for _, flag := range flags {
		if err := cmd.Flags().MarkHidden(flag); err != nil {
			panic(err)
		}
	}
}

func partitionTypeIDToName(partitionTypeID types.PartitionTypeID) string {
	switch partitionTypeID {
	case 1:
		return "money"
	case 2:
		return "tokens"
	case 3:
		return "evm"
	case 4:
		return "orchestration"
	default:
		return "unknown"
	}
}
