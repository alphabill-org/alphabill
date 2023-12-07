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

	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/util"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	BoltBlockStoreFileName = "blocks.db"
)

type baseNodeConfiguration struct {
	Base *baseConfiguration
}

type startNodeConfiguration struct {
	Address                    string
	Genesis                    string
	StateFile                  string
	KeyFile                    string
	DbFile                     string
	TxIndexerDBFile            string
	LedgerReplicationMaxBlocks uint64
	LedgerReplicationMaxTx     uint32
	BootStrapAddresses         string // boot strap addresses (libp2p multiaddress format)
}

func run(ctx context.Context, name string, node *partition.Node, rpcServerConf *grpcServerConfiguration, restServerConf *restServerConfiguration,
	proofStore keyvaluedb.KeyValueDB, obs Observability) error {
	log := obs.Logger()
	log.InfoContext(ctx, fmt.Sprintf("starting %s: BuildInfo=%s", name, debug.ReadBuildInfo()))

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return node.Run(ctx) })

	g.Go(func() error {
		grpcServer, err := initRPCServer(node, rpcServerConf, obs, log)
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
		routers := []rpc.Registrar{
			rpc.NodeEndpoints(node, proofStore, obs, log),
			rpc.MetricsEndpoints(obs.PrometheusRegisterer()),
			rpc.InfoEndpoints(node, name, node.GetPeer(), log),
		}
		if restServerConf.router != nil {
			routers = append(routers, restServerConf.router)
		}
		restServer := initRESTServer(restServerConf, obs, log, routers...)

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

func initRESTServer(conf *restServerConfiguration, obs partition.Observability, log *slog.Logger, routes ...rpc.Registrar) *http.Server {
	rs := rpc.NewRESTServer(conf.Address, conf.MaxBodyBytes, obs, log, routes...)
	rs.ReadTimeout = conf.ReadTimeout
	rs.ReadHeaderTimeout = conf.ReadHeaderTimeout
	rs.WriteTimeout = conf.WriteTimeout
	rs.IdleTimeout = conf.IdleTimeout
	rs.MaxHeaderBytes = conf.MaxHeaderBytes
	return rs
}

func loadPeerConfiguration(keys *Keys, pg *genesis.PartitionGenesis, cfg *startNodeConfiguration) (*network.PeerConfiguration, error) {
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
	bootNodes, err := getBootStrapNodes(cfg.BootStrapAddresses)
	if err != nil {
		return nil, fmt.Errorf("boot nodes parameter error: %w", err)
	}
	return network.NewPeerConfiguration(cfg.Address, pair, bootNodes, validatorIdentifiers)
}

func initRPCServer(node *partition.Node, cfg *grpcServerConfiguration, obs partition.Observability, log *slog.Logger) (*grpc.Server, error) {
	grpcServer := grpc.NewServer(
		grpc.MaxSendMsgSize(cfg.MaxSendMsgSize),
		grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize),
		grpc.KeepaliveParams(cfg.GrpcKeepAliveServerParameters()),
		grpc.UnaryInterceptor(rpc.InstrumentMetricsUnaryServerInterceptor(obs.Meter(rpc.MetricsScopeGRPCAPI), log)),
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(obs.TracerProvider()))),
	)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

	rpcServer, err := rpc.NewGRPCServer(node, obs, rpc.WithMaxGetBlocksBatchSize(cfg.MaxGetBlocksBatchSize))
	if err != nil {
		return nil, err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

func createNode(ctx context.Context, txs txsystem.TransactionSystem, cfg *startNodeConfiguration, keys *Keys,
	blockStore keyvaluedb.KeyValueDB, proofStore keyvaluedb.KeyValueDB, obs Observability) (*partition.Node, error) {
	pg, err := loadPartitionGenesis(cfg.Genesis)
	if err != nil {
		return nil, err
	}
	// Load network configuration. In testnet, we assume that all validators know the address of all other validators.
	peerConf, err := loadPeerConfiguration(keys, pg, cfg)
	if err != nil {
		return nil, err
	}
	if len(pg.RootValidators) < 1 {
		return nil, errors.New("root validator info is missing")
	}
	if blockStore == nil {
		blockStore, err = initStore(cfg.DbFile)
		if err != nil {
			return nil, err
		}
	}

	options := []partition.NodeOption{
		partition.WithBlockStore(blockStore),
		partition.WithReplicationParams(cfg.LedgerReplicationMaxBlocks, cfg.LedgerReplicationMaxTx),
		partition.WithProofIndex(proofStore, 20),
		// TODO history size!
	}

	node, err := partition.NewNode(
		ctx,
		peerConf,
		keys.SigningPrivateKey,
		txs,
		pg,
		nil,
		obs,
		options...,
	)
	if err != nil {
		return nil, err
	}
	return node, nil
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

func initStore(dbFile string) (keyvaluedb.KeyValueDB, error) {
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
	nodeCmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", fmt.Sprintf("path to the key file (default: $AB_HOME/%s/keys.json)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.Genesis, "genesis", "g", "", fmt.Sprintf("path to the partition genesis file : $AB_HOME/%s/partition-genesis.json)", partitionSuffix))
	nodeCmd.Flags().StringVar(&config.BootStrapAddresses, rootBootStrapNodesCmdFlag, "", "comma separated list of bootstrap root node addresses id@libp2p-multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.DbFile, "db", "f", "", fmt.Sprintf("path to the database file (default: $AB_HOME/%s/%s)", partitionSuffix, BoltBlockStoreFileName))
	nodeCmd.Flags().StringVarP(&config.TxIndexerDBFile, "tx-db", "", "", "path to the transaction indexer database file")
	nodeCmd.Flags().Uint64Var(&config.LedgerReplicationMaxBlocks, "ledger-replication-max-blocks", 1000, "maximum number of blocks to return in a single replication response")
	nodeCmd.Flags().Uint32Var(&config.LedgerReplicationMaxTx, "ledger-replication-max-transactions", 10000, "maximum number of transactions to return in a single replication response")
}
