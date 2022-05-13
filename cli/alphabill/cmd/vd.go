package cmd

import (
	"bytes"
	"context"
	"net"
	"sort"

	"github.com/multiformats/go-multiaddr"

	"github.com/libp2p/go-libp2p-core/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/verifiable_data"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type (
	vdConfiguration struct {
		baseNodeConfiguration
		Address          string
		Peers            map[string]string
		Genesis          string
		KeyFile          string
		RootChainAddress string
	}
)

func (c vdConfiguration) getPeerAddress(identifier string) (string, error) {
	address, f := c.Peers[identifier]
	if !f {
		return "", errors.Errorf("address for node %v not found.", identifier)
	}
	return address, nil
}

func newVDNodeCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &vdConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base:   baseConfig,
			Server: &grpcServerConfiguration{},
		},
	}

	var nodeCmd = &cobra.Command{
		Use:   "vd",
		Short: "Starts a Verifiable Data partition's node",
		Long:  `Starts a Verifiable Data partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return defaultNodeRunFunc(ctx, config)
		},
	}

	nodeCmd.Flags().StringVarP(&config.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "node address in libp2p multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.RootChainAddress, "rootchain", "r", "/ip4/127.0.0.1/tcp/26662", "root chain address in libp2p multiaddress-format")
	nodeCmd.Flags().StringToStringVarP(&config.Peers, "peers", "p", nil, "a map of partition peer identifiers and addresses. must contain all genesis validator addresses")
	nodeCmd.Flags().StringVarP(&config.KeyFile, keyFileCmd, "k", "", "path to the key file (default: $AB_HOME/vd/keys.json)")
	nodeCmd.Flags().StringVarP(&config.Genesis, "genesis", "g", "", "path to the partition genesis file : $AB_HOME/vd/partition-genesis.json)")

	config.Server.addConfigurationFlags(nodeCmd)

	return nodeCmd
}

func defaultNodeRunFunc(ctx context.Context, cfg *vdConfiguration) error {
	node, err := startNode(ctx, cfg)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Server.Address)
	if err != nil {
		return err
	}
	grpcServer, err := initRPCServer(cfg, node)
	if err != nil {
		return err
	}
	starterFunc := func(ctx context.Context) {
		go func() {
			log.Info("Starting gRPC server on %s", cfg.Server.Address)
			err = grpcServer.Serve(listener)
			if err != nil {
				log.Error("Server exited with erroneous situation: %s", err)
				return
			}
			log.Info("Server exited successfully")
		}()
		<-ctx.Done()
		grpcServer.GracefulStop()
		node.Close()
	}

	return starter.StartAndWait(ctx, "vd node", starterFunc)
}

func initRPCServer(cfg *vdConfiguration, node *partition.Node) (*grpc.Server, error) {
	grpcServer := grpc.NewServer(
		grpc.MaxSendMsgSize(cfg.Server.MaxRecvMsgSize),
		grpc.KeepaliveParams(cfg.Server.GrpcKeepAliveServerParameters()),
	)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

	rpcServer, err := rpc.NewRpcServer2(node)
	if err != nil {
		return nil, err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

func startNode(ctx context.Context, cfg *vdConfiguration) (*partition.Node, error) {
	keys, err := LoadKeys(cfg.KeyFile, false)
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
	txSystem, err := verifiable_data.New()
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
	// TODO use boltDB block store after node recovery is implemented
	node, err := partition.New(
		p,
		keys.SigningPrivateKey,
		txSystem,
		pg,
		partition.WithContext(ctx),
		partition.WithDefaultEventProcessors(true),
		partition.WithRootAddressAndIdentifier(newMultiAddr, rootID),
	)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func loadNetworkConfiguration(keys *Keys, pg *genesis.PartitionGenesis, cfg *vdConfiguration) (*network.Peer, error) {
	pair, err := getEncryptionKeyPair(keys)
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

func getEncryptionKeyPair(keys *Keys) (*network.PeerKeyPair, error) {
	private, err := keys.EncryptionPrivateKey.Raw()
	if err != nil {
		return nil, err
	}
	public, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return nil, err
	}

	return &network.PeerKeyPair{
		PublicKey:  public,
		PrivateKey: private,
	}, nil
}

func loadPartitionGenesis(genesisPath string) (*genesis.PartitionGenesis, error) {
	pg, err := util.ReadJsonFile(genesisPath, &genesis.PartitionGenesis{})
	if err != nil {
		return nil, err
	}
	return pg, nil
}
