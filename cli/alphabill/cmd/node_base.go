package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rpc"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

const (
	BoltBlockStoreFileName = "blocks.db"
	TxIndexerStoreFileName = "tx_indexer.db"
)

type baseNodeConfiguration struct {
	Base *baseConfiguration
}

type startNodeConfiguration struct {
	Address                    string
	Genesis                    string
	KeyFile                    string
	RootChainAddress           string
	DbFile                     string
	TxIndexerDBFile            string
	LedgerReplicationMaxBlocks uint64
	LedgerReplicationMaxTx     uint32
}

func defaultNodeRunFunc(ctx context.Context, name string, txs txsystem.TransactionSystem, nodeCfg *startNodeConfiguration, rpcServerConf *grpcServerConfiguration, restServerConf *restServerConfiguration, log *slog.Logger) error {
	self, node, err := createNode(ctx, txs, nodeCfg, nil, log)
	if err != nil {
		return fmt.Errorf("creating %q node: %w", name, err)
	}
	return run(ctx, name, self, node, rpcServerConf, restServerConf, log.With(logger.NodeID(self.ID())))
}

func run(ctx context.Context, name string, self *network.Peer, node *partition.Node, rpcServerConf *grpcServerConfiguration, restServerConf *restServerConfiguration, log *slog.Logger) error {
	log.InfoContext(ctx, fmt.Sprintf("starting %s: BuildInfo=%s", name, debug.ReadBuildInfo()))
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return node.Run(ctx) })

	g.Go(func() error {
		grpcServer, err := initRPCServer(node, rpcServerConf)
		if err != nil {
			return fmt.Errorf("failed to init gRPC server for %s: %w", name, err)
		}

		listener, err := net.Listen("tcp", rpcServerConf.Address)
		if err != nil {
			return fmt.Errorf("failed to open listener on %q for %s: %w", rpcServerConf.Address, name, err)
		}

		errch := make(chan error, 1)
		go func() {
			log.Info("%s gRPC server starting on %s", name, rpcServerConf.Address)
			if err := grpcServer.Serve(listener); err != nil {
				errch <- fmt.Errorf("%s gRPC server exited: %w", name, err)
			}
			log.InfoContext(ctx, fmt.Sprintf("%s gRPC server exited", name))
		}()

		select {
		case <-ctx.Done():
			grpcServer.GracefulStop()
			return ctx.Err()
		case err := <-errch:
			return err
		}
	})

	g.Go(func() error {
		if restServerConf.IsAddressEmpty() {
			return nil // return nil in this case in order not to kill the group!
		}
		routers := []rpc.Registrar{rpc.NodeEndpoints(node), rpc.MetricsEndpoints(), rpc.InfoEndpoints(node, name, self)}
		if restServerConf.router != nil {
			routers = append(routers, restServerConf.router)
		}
		restServer := initRESTServer(restServerConf, routers...)

		errch := make(chan error, 1)
		go func() {
			log.Info("%s REST server starting on %s", name, restServer.Addr)
			if err := restServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errch <- fmt.Errorf("%s REST server exited: %w", name, err)
			}
			log.InfoContext(ctx, fmt.Sprintf("%s REST server exited", name))
		}()

		select {
		case <-ctx.Done():
			err := ctx.Err()
			if e := restServer.Close(); e != nil {
				err = errors.Join(err, fmt.Errorf("closing REST server: %w", e))
			}
			return err
		case err := <-errch:
			return err
		}
	})

	return g.Wait()
}

func initRESTServer(conf *restServerConfiguration, routes ...rpc.Registrar) *http.Server {
	rs := rpc.NewRESTServer(conf.Address, conf.MaxBodyBytes, routes...)
	rs.ReadTimeout = conf.ReadTimeout
	rs.ReadHeaderTimeout = conf.ReadHeaderTimeout
	rs.WriteTimeout = conf.WriteTimeout
	rs.IdleTimeout = conf.IdleTimeout
	rs.MaxHeaderBytes = conf.MaxHeaderBytes
	return rs
}

func loadNetworkConfiguration(ctx context.Context, keys *Keys, pg *genesis.PartitionGenesis, cfg *startNodeConfiguration, log *slog.Logger) (*network.Peer, error) {
	pair, err := keys.getEncryptionKeyPair()
	if err != nil {
		return nil, err
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return nil, err
	}
	validatorIdentifiers := make(peer.IDSlice, len(pg.Keys))
	for i, k := range pg.Keys {
		if peerID.String() == k.NodeIdentifier {
			if !bytes.Equal(pair.PublicKey, k.EncryptionPublicKey) {
				return nil, fmt.Errorf("invalid encryption key: expected %X, got %X", pair.PublicKey, k.EncryptionPublicKey)
			}
		}
		nodeID, err := network.NodeIDFromPublicKeyBytes(k.EncryptionPublicKey)
		if err != nil {
			return nil, fmt.Errorf("invalid encryption key: %X", k.EncryptionPublicKey)
		}
		if nodeID.String() != k.NodeIdentifier {
			return nil, fmt.Errorf("invalid nodeID/encryption key combination: %s", nodeID)
		}
		validatorIdentifiers[i] = nodeID
	}
	sort.Sort(validatorIdentifiers)

	// Assume monolithic root chain for now and only extract the id of the first root node.
	// Assume monolithic root chain is also a bootstrap node.
	bootstrapNodeID, bootstrapNodeAddress, err := getRootValidatorIDAndMultiAddress(pg.RootValidators[0].EncryptionPublicKey, cfg.RootChainAddress)
	if err != nil {
		return nil, err
	}

	peerConfiguration := &network.PeerConfiguration{
		Address:        cfg.Address,
		KeyPair:        pair,
		BootstrapPeers: []peer.AddrInfo{{ID: bootstrapNodeID, Addrs: []multiaddr.Multiaddr{bootstrapNodeAddress}}},
		Validators:     validatorIdentifiers,
	}

	p, err := network.NewPeer(ctx, peerConfiguration, log)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func initRPCServer(node *partition.Node, cfg *grpcServerConfiguration) (*grpc.Server, error) {
	grpcServer := grpc.NewServer(
		grpc.MaxSendMsgSize(cfg.MaxSendMsgSize),
		grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize),
		grpc.KeepaliveParams(cfg.GrpcKeepAliveServerParameters()),
	)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

	rpcServer, err := rpc.NewGRPCServer(node, rpc.WithMaxGetBlocksBatchSize(cfg.MaxGetBlocksBatchSize))
	if err != nil {
		return nil, err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

func createNode(ctx context.Context, txs txsystem.TransactionSystem, cfg *startNodeConfiguration, blockStore keyvaluedb.KeyValueDB, log *slog.Logger) (*network.Peer, *partition.Node, error) {
	keys, err := LoadKeys(cfg.KeyFile, false, false)
	if err != nil {
		return nil, nil, err
	}
	pg, err := loadPartitionGenesis(cfg.Genesis)
	if err != nil {
		return nil, nil, err
	}
	// Load network configuration. In testnet, we assume that all validators know the address of all other validators.
	p, err := loadNetworkConfiguration(ctx, keys, pg, cfg, log)
	if err != nil {
		return nil, nil, err
	}
	log = log.With(logger.NodeID(p.ID()))

	if len(pg.RootValidators) < 1 {
		return nil, nil, errors.New("root validator info is missing")
	}
	// Assume monolithic root chain for now and only extract the id of the first root node
	rootValidatorEncryptionKey := pg.RootValidators[0].EncryptionPublicKey
	rootID, rootAddress, err := getRootValidatorIDAndMultiAddress(rootValidatorEncryptionKey, cfg.RootChainAddress)
	if err != nil {
		return nil, nil, err
	}
	n, err := network.NewLibP2PValidatorNetwork(p, network.DefaultValidatorNetOptions, log)
	if err != nil {
		return nil, nil, err
	}
	if blockStore == nil {
		blockStore, err = initNodeBlockStore(cfg.DbFile)
		if err != nil {
			return nil, nil, err
		}
	}
	options := []partition.NodeOption{
		partition.WithRootAddressAndIdentifier(rootAddress, rootID),
		partition.WithBlockStore(blockStore),
		partition.WithReplicationParams(cfg.LedgerReplicationMaxBlocks, cfg.LedgerReplicationMaxTx),
	}

	if cfg.TxIndexerDBFile != "" {
		txIndexer, err := boltdb.New(cfg.TxIndexerDBFile)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to load tx indexer: %w", err)
		}
		options = append(options, partition.WithTxIndexer(txIndexer))
	}

	node, err := partition.New(
		p,
		keys.SigningPrivateKey,
		txs,
		pg,
		n,
		log,
		options...,
	)
	if err != nil {
		return nil, nil, err
	}
	return p, node, nil
}

func getRootValidatorIDAndMultiAddress(rootValidatorEncryptionKey []byte, addressStr string) (peer.ID, multiaddr.Multiaddr, error) {
	rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(rootValidatorEncryptionKey)
	if err != nil {
		return "", nil, err
	}
	rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
	if err != nil {
		return "", nil, err
	}
	rootAddress, err := multiaddr.NewMultiaddr(addressStr)
	if err != nil {
		return "", nil, err
	}
	return rootID, rootAddress, nil
}

func initNodeBlockStore(dbFile string) (keyvaluedb.KeyValueDB, error) {
	if dbFile != "" {
		return boltdb.New(dbFile)
	}
	return memorydb.New(), nil
}

func loadPartitionGenesis(genesisPath string) (*genesis.PartitionGenesis, error) {
	pg, err := util.ReadJsonFile(genesisPath, &genesis.PartitionGenesis{})
	if err != nil {
		return nil, err
	}
	return pg, nil
}

func addCommonNodeConfigurationFlags(nodeCmd *cobra.Command, config *startNodeConfiguration, partitionSuffix string) {
	nodeCmd.Flags().StringVarP(&config.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "node address in libp2p multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.RootChainAddress, "rootchain", "r", "/ip4/127.0.0.1/tcp/26662", "root chain address in libp2p multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", fmt.Sprintf("path to the key file (default: $AB_HOME/%s/keys.json)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.Genesis, "genesis", "g", "", fmt.Sprintf("path to the partition genesis file : $AB_HOME/%s/partition-genesis.json)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.DbFile, "db", "f", "", fmt.Sprintf("path to the database file (default: $AB_HOME/%s/%s)", partitionSuffix, BoltBlockStoreFileName))
	nodeCmd.Flags().StringVarP(&config.TxIndexerDBFile, "tx-db", "", "", "path to the transaction indexer database file")
	nodeCmd.Flags().Uint64Var(&config.LedgerReplicationMaxBlocks, "ledger-replication-max-blocks", 1000, "maximum number of blocks to return in a single replication response")
	nodeCmd.Flags().Uint32Var(&config.LedgerReplicationMaxTx, "ledger-replication-max-transactions", 10000, "maximum number of transactions to return in a single replication response")
}
