package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sort"

	"github.com/alphabill-org/alphabill/internal/async"
	"github.com/alphabill-org/alphabill/internal/async/future"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/partition/store"
	"github.com/alphabill-org/alphabill/internal/rpc"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/starter"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type baseNodeConfiguration struct {
	Base *baseConfiguration
}

type startNodeConfiguration struct {
	Address                    string
	Peers                      map[string]string
	Genesis                    string
	KeyFile                    string
	RootChainAddress           string
	DbFile                     string
	LedgerReplicationMaxBlocks uint64
	LedgerReplicationMaxTx     uint32
}

func defaultNodeRunFunc(ctx context.Context, name string, txs txsystem.TransactionSystem, nodeCfg *startNodeConfiguration, rpcServerConf *grpcServerConfiguration, restServerConf *restServerConfiguration) error {
	self, node, err := startNode(ctx, txs, nodeCfg)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", rpcServerConf.Address)
	if err != nil {
		return err
	}
	grpcServer, err := initRPCServer(node, rpcServerConf)
	if err != nil {
		return err
	}
	restServer, err := initRESTServer(node, self, restServerConf)
	if err != nil {
		return err
	}
	starterFunc := func(ctx context.Context) {
		async.MakeWorker("grpc transport layer server", func(ctx context.Context) future.Value {
			go func() {
				log.Info("Starting gRPC server on %s", rpcServerConf.Address)
				err = grpcServer.Serve(listener)
				if err != nil {
					log.Error("gRPC Server exited with erroneous situation: %s", err)
				} else {
					log.Info("gRPC Server exited successfully")
				}
			}()
			if restServer != nil {
				go func() {
					log.Info("Starting REST server on %s", restServer.Addr)
					err = restServer.ListenAndServe()
					if err != nil {
						log.Error("REST server exited with erroneous situation: %s", err)
					} else {
						log.Info("REST server exited successfully")
					}
				}()
			}
			<-ctx.Done()
			grpcServer.GracefulStop()
			if restServer != nil {
				e := restServer.Close()
				if e != nil {
					log.Error("Failed to close REST server: %v", err)
				}
			}
			node.Close()
			return nil
		}).Start(ctx)
	}
	// StartAndWait waits until ctx.waitgroup is done OR sigterm cancels signal OR timeout (not used here)
	return starter.StartAndWait(ctx, name, starterFunc)
}

func initRESTServer(node *partition.Node, self *network.Peer, conf *restServerConfiguration) (*rpc.RestServer, error) {
	if conf.IsAddressEmpty() {
		// Address not configured.
		return nil, nil
	}
	rs, err := rpc.NewRESTServer(node, conf.Address, conf.MaxBodyBytes, self)
	if err != nil {
		return nil, err
	}
	rs.ReadTimeout = conf.ReadTimeout
	rs.ReadHeaderTimeout = conf.ReadHeaderTimeout
	rs.WriteTimeout = conf.WriteTimeout
	rs.IdleTimeout = conf.IdleTimeout
	rs.MaxHeaderBytes = conf.MaxHeaderBytes
	return rs, nil
}

func loadNetworkConfiguration(keys *Keys, pg *genesis.PartitionGenesis, cfg *startNodeConfiguration) (*network.Peer, error) {
	pair, err := keys.getEncryptionKeyPair()
	if err != nil {
		return nil, err
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return nil, err
	}
	var persistentPeers = make([]*network.PeerInfo, len(pg.Keys))
	for i, key := range pg.Keys {
		if peerID.String() == key.NodeIdentifier {
			if !bytes.Equal(pair.PublicKey, key.EncryptionPublicKey) {
				return nil, errors.New("invalid encryption key")
			}
			persistentPeers[i] = &network.PeerInfo{
				Address:   cfg.Address,
				PublicKey: key.EncryptionPublicKey,
			}
			continue
		}

		peerAddress, err := cfg.getPeerAddress(key.NodeIdentifier)
		if err != nil {
			return nil, err
		}

		persistentPeers[i] = &network.PeerInfo{
			Address:   peerAddress,
			PublicKey: key.EncryptionPublicKey,
		}
	}

	sort.Slice(persistentPeers, func(i, j int) bool {
		return string(persistentPeers[i].PublicKey) < string(persistentPeers[j].PublicKey)
	})

	peerConfiguration := &network.PeerConfiguration{
		Address:         cfg.Address,
		KeyPair:         pair,
		PersistentPeers: persistentPeers,
	}

	p, err := network.NewPeer(peerConfiguration)
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

func startNode(ctx context.Context, txs txsystem.TransactionSystem, cfg *startNodeConfiguration) (*network.Peer, *partition.Node, error) {
	keys, err := LoadKeys(cfg.KeyFile, false, false)
	if err != nil {
		return nil, nil, err
	}
	pg, err := loadPartitionGenesis(cfg.Genesis)
	if err != nil {
		return nil, nil, err
	}
	// Load network configuration. In testnet, we assume that all validators know the address of all other validators.
	p, err := loadNetworkConfiguration(keys, pg, cfg)
	if err != nil {
		return nil, nil, err
	}
	if len(pg.RootValidators) < 1 {
		return nil, nil, errors.New("Root validator info is missing")
	}
	// Assume monolithic root chain for now and only extract the id of the first root node
	rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(pg.RootValidators[0].EncryptionPublicKey)
	if err != nil {
		return nil, nil, err
	}
	rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
	if err != nil {
		return nil, nil, err
	}
	newMultiAddr, err := multiaddr.NewMultiaddr(cfg.RootChainAddress)
	if err != nil {
		return nil, nil, err
	}
	n, err := network.NewLibP2PValidatorNetwork(p, network.DefaultValidatorNetOptions)
	if err != nil {
		return nil, nil, err
	}
	blockStore, err := initNodeBlockStore(cfg.DbFile)
	if err != nil {
		return nil, nil, err
	}
	node, err := partition.New(
		p,
		keys.SigningPrivateKey,
		txs,
		pg,
		n,
		partition.WithContext(ctx),
		partition.WithRootAddressAndIdentifier(newMultiAddr, rootID),
		partition.WithBlockStore(blockStore),
	)
	if err != nil {
		return nil, nil, err
	}
	return p, node, nil
}

func initNodeBlockStore(dbFile string) (store.BlockStore, error) {
	if dbFile != "" {
		return store.NewBoltBlockStore(dbFile)
	} else {
		return store.NewInMemoryBlockStore(), nil
	}
}

func loadPartitionGenesis(genesisPath string) (*genesis.PartitionGenesis, error) {
	pg, err := util.ReadJsonFile(genesisPath, &genesis.PartitionGenesis{})
	if err != nil {
		return nil, err
	}
	return pg, nil
}

func (c *startNodeConfiguration) getPeerAddress(identifier string) (string, error) {
	address, f := c.Peers[identifier]
	if !f {
		return "", errors.Errorf("address for node %v not found.", identifier)
	}
	return address, nil
}

func addCommonNodeConfigurationFlags(nodeCmd *cobra.Command, config *startNodeConfiguration, partitionSuffix string) {
	nodeCmd.Flags().StringVarP(&config.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "node address in libp2p multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.RootChainAddress, "rootchain", "r", "/ip4/127.0.0.1/tcp/26662", "root chain address in libp2p multiaddress-format")
	nodeCmd.Flags().StringToStringVarP(&config.Peers, "peers", "p", nil, "a map of partition peer identifiers and addresses. must contain all genesis validator addresses")
	nodeCmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", fmt.Sprintf("path to the key file (default: $AB_HOME/%s/keys.json)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.Genesis, "genesis", "g", "", fmt.Sprintf("path to the partition genesis file : $AB_HOME/%s/partition-genesis.json)", partitionSuffix))
	nodeCmd.Flags().StringVarP(&config.DbFile, "db", "f", "", fmt.Sprintf("path to the database file (default: $AB_HOME/%s/%s)", partitionSuffix, store.BoltBlockStoreFileName))
	nodeCmd.Flags().Uint64Var(&config.LedgerReplicationMaxBlocks, "ledger-replication-max-blocks", 1000, "maximum number of blocks to return in a single replication response")
	nodeCmd.Flags().Uint32Var(&config.LedgerReplicationMaxTx, "ledger-replication-max-transactions", 10000, "maximum number of transactions to return in a single replication response")
}
