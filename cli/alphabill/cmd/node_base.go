package cmd

import (
	"bytes"
	"context"
	"net"
	"sort"

	"github.com/alphabill-org/alphabill/internal/partition/store"

	"github.com/alphabill-org/alphabill/internal/async"
	"github.com/alphabill-org/alphabill/internal/async/future"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rpc"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/starter"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type baseNodeConfiguration struct {
	Base *baseConfiguration
}

type startNodeConfiguration struct {
	Address          string
	Peers            map[string]string
	Genesis          string
	KeyFile          string
	RootChainAddress string
	DbFile           string
}

func defaultNodeRunFunc(ctx context.Context, name string, txs txsystem.TransactionSystem, nodeCfg *startNodeConfiguration, rpcServerConf *grpcServerConfiguration) error {
	node, err := startNode(ctx, txs, nodeCfg)
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
	starterFunc := func(ctx context.Context) {
		async.MakeWorker("grpc transport layer server", func(ctx context.Context) future.Value {
			go func() {
				log.Info("Starting gRPC server on %s", rpcServerConf.Address)
				err = grpcServer.Serve(listener)
				if err != nil {
					log.Error("Server exited with erroneous situation: %s", err)
				} else {
					log.Info("Server exited successfully")
				}
			}()
			<-ctx.Done()
			grpcServer.GracefulStop()
			node.Close()
			return nil
		}).Start(ctx)
	}
	// StartAndWait waits until ctx.waitgroup is done OR sigterm cancels signal OR timeout (not used here)
	return starter.StartAndWait(ctx, name, starterFunc)
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
		grpc.MaxSendMsgSize(cfg.MaxRecvMsgSize),
		grpc.KeepaliveParams(cfg.GrpcKeepAliveServerParameters()),
	)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

	rpcServer, err := rpc.NewRpcServer(node, rpc.WithMaxGetBlocksBatchSize(cfg.MaxGetBlocksBatchSize))
	if err != nil {
		return nil, err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

func startNode(ctx context.Context, txs txsystem.TransactionSystem, cfg *startNodeConfiguration) (*partition.Node, error) {
	keys, err := LoadKeys(cfg.KeyFile, false, false)
	if err != nil {
		return nil, err
	}
	pg, err := loadPartitionGenesis(cfg.Genesis)
	if err != nil {
		return nil, err
	}
	// Load network configuration. In testnet, we assume that all validators know the address of all other validators.
	p, err := loadNetworkConfiguration(keys, pg, cfg)
	if err != nil {
		return nil, err
	}

	rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(pg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
	if err != nil {
		return nil, err
	}
	newMultiAddr, err := multiaddr.NewMultiaddr(cfg.RootChainAddress)
	if err != nil {
		return nil, err
	}
	n, err := network.NewLibP2PValidatorNetwork(p, network.DefaultValidatorNetOptions)
	if err != nil {
		return nil, err
	}
	blockStore, err := initBlockStore(cfg.DbFile)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return node, nil
}

func initBlockStore(dbFile string) (store.BlockStore, error) {
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
