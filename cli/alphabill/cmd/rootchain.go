package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ainvaltin/httpsrv"
	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/abdrc"
	"github.com/alphabill-org/alphabill/rootchain/consensus/monolithic"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/util"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

const (
	boltRootChainStoreFileName = "rootchain.db"
	rootPortCmdFlag            = "root-listener"
	rootBootStrapNodesCmdFlag  = "bootnodes"
	defaultNetworkTimeout      = 300 * time.Millisecond
)

type rootNodeConfig struct {
	Base               *baseConfiguration
	KeyFile            string // path to rootchain chain key file
	GenesisFile        string // path to rootchain-genesis.json file
	Address            string // node address (libp2p multiaddress format)
	StoragePath        string // path to Bolt storage file
	MaxRequests        uint   // validator partition certification request channel capacity
	BootStrapAddresses string // boot strap addresses (libp2p multiaddress format)
	RPCServerAddress   string // address on which http server is exposed with metrics endpoint
}

// newRootNodeCmd creates a new cobra command for root chain node
func newRootNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &rootNodeConfig{
		Base: baseConfig,
	}
	var cmd = &cobra.Command{
		Use:   "root",
		Short: "Starts a root chain node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRootNode(cmd.Context(), config)
		},
	}

	cmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", "path to node validator key file  (default $AB_HOME/rootchain/"+defaultKeysFileName+")")
	cmd.Flags().StringVar(&config.GenesisFile, "genesis-file", "", "path to root-genesis.json file (default $AB_HOME/rootchain/"+rootGenesisFileName+")")
	cmd.Flags().StringVar(&config.StoragePath, "db", "", "persistent store path (default: $AB_HOME/rootchain/)")
	cmd.Flags().StringVar(&config.Address, "address", "/ip4/127.0.0.1/tcp/26662", "validator address in libp2p multiaddress-format")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "request buffer capacity")
	cmd.Flags().StringVar(&config.BootStrapAddresses, rootBootStrapNodesCmdFlag, "", "comma separated list of bootstrap root node addresses id@libp2p-multiaddress-format")
	cmd.Flags().StringVar(&config.RPCServerAddress, "rpc-server-address", "", `Specifies the TCP address for the RPC server to listen on, in the form "host:port". RPC server isn't initialised if address is empty.`)
	return cmd
}

// splitAndTrim splits input separated by a comma and trims excessive white space from the substrings.
func splitAndTrim(input string) (ret []string) {
	l := strings.Split(input, ",")
	for _, r := range l {
		if r = strings.TrimSpace(r); r != "" {
			ret = append(ret, r)
		}
	}
	return ret
}

// getGenesisFilePath returns genesis file path if provided, otherwise $AB_HOME/rootchain/root-genesis.json
// Must be called after $AB_HOME is initialized in base command PersistentPreRunE function.
func (c *rootNodeConfig) getGenesisFilePath() string {
	if c.GenesisFile != "" {
		return c.GenesisFile
	}
	return filepath.Join(c.Base.defaultRootGenesisDir(), rootGenesisFileName)
}

func (c *rootNodeConfig) getStoragePath() string {
	if c.StoragePath != "" {
		return c.StoragePath
	}
	return c.Base.defaultRootGenesisDir()
}

func (c *rootNodeConfig) getKeyFilePath() string {
	if c.KeyFile != "" {
		return c.KeyFile
	}
	return filepath.Join(c.Base.defaultRootGenesisDir(), defaultKeysFileName)
}

func getBootStrapNodes(bootNodesStr string) ([]peer.AddrInfo, error) {
	if bootNodesStr == "" {
		return []peer.AddrInfo{}, nil
	}
	nodeStrings := splitAndTrim(bootNodesStr)
	bootNodes := make([]peer.AddrInfo, len(nodeStrings))
	for i, str := range nodeStrings {
		l := strings.Split(str, "@")
		if len(l) != 2 {
			return nil, fmt.Errorf("invalid bootstrap node parameter: %s", str)
		}
		id, err := peer.Decode(l[0])
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap node id: %s", l[0])
		}
		addr, err := ma.NewMultiaddr(l[1])
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap node address: %s", l[1])
		}
		bootNodes[i].ID = id
		bootNodes[i].Addrs = []ma.Multiaddr{addr}
	}
	return bootNodes, nil
}

func initRootStore(dbPath string) (keyvaluedb.KeyValueDB, error) {
	if dbPath != "" {
		return boltdb.New(filepath.Join(dbPath, boltRootChainStoreFileName))
	}
	return nil, fmt.Errorf("persistent storage path not set")
}

func runRootNode(ctx context.Context, config *rootNodeConfig) error {
	rootGenesis, err := util.ReadJsonFile(config.getGenesisFilePath(), &genesis.RootGenesis{})
	if err != nil {
		return fmt.Errorf("loading root node genesis file %s: %w", config.getGenesisFilePath(), err)
	}
	keys, err := LoadKeys(config.getKeyFilePath(), false, false)
	if err != nil {
		return fmt.Errorf("loading keys from %s: %w", config.KeyFile, err)
	}
	// check if genesis file is valid and exit early if is not
	if err = rootGenesis.Verify(); err != nil {
		return fmt.Errorf("root genesis verification failed: %w", err)
	}
	// Process partition node network
	host, err := createHost(ctx, keys, config)
	if err != nil {
		return fmt.Errorf("creating partition host: %w", err)
	}
	log := config.Base.observe.Logger().With(logger.NodeID(host.ID()))
	obs := observability.WithLogger(config.Base.observe, log)
	partitionNet, err := network.NewLibP2PRootChainNetwork(host, config.MaxRequests, defaultNetworkTimeout, obs)
	if err != nil {
		return fmt.Errorf("partition network initialization failed: %w", err)
	}
	ver, err := keys.SigningPrivateKey.Verifier()
	if err != nil {
		return fmt.Errorf("invalid root node sign key: %w", err)
	}
	if err = verifyKeyPresentInGenesis(host, rootGenesis.Root, ver); err != nil {
		return fmt.Errorf("root node key not found in genesis: %w", err)
	}
	// initiate partition store
	partitionCfg, err := partitions.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	if err != nil {
		return fmt.Errorf("failed to extract partition info from genesis file %s: %w", config.getGenesisFilePath(), err)
	}
	// Initiate root storage
	store, err := initRootStore(config.getStoragePath())
	if err != nil {
		return fmt.Errorf("root store init failed: %w", err)
	}
	var cm rootchain.ConsensusManager
	if len(rootGenesis.Root.RootValidators) == 1 {
		// use monolithic consensus algorithm
		cm, err = monolithic.NewMonolithicConsensusManager(
			host.ID().String(),
			rootGenesis,
			partitionCfg,
			keys.SigningPrivateKey,
			log,
			consensus.WithStorage(store))
	} else {
		var rootNet *network.LibP2PNetwork
		rootNet, err = network.NewLibP2RootConsensusNetwork(host, config.MaxRequests, defaultNetworkTimeout, obs)
		if err != nil {
			return fmt.Errorf("failed initiate root network, %w", err)
		}
		// Create distributed consensus manager function
		cm, err = abdrc.NewDistributedAbConsensusManager(host.ID(),
			rootGenesis,
			partitionCfg,
			rootNet,
			keys.SigningPrivateKey,
			obs,
			consensus.WithStorage(store))
	}
	if err != nil {
		return fmt.Errorf("failed initiate consensus manager: %w", err)
	}
	node, err := rootchain.New(
		host,
		partitionNet,
		partitionCfg,
		cm,
		obs,
	)
	if err != nil {
		return fmt.Errorf("failed initiate root node: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return node.Run(ctx) })

	g.Go(func() error {
		pr := config.Base.observe.PrometheusRegisterer()
		if pr == nil || config.RPCServerAddress == "" {
			return nil // do not kill the group!
		}

		mux := http.NewServeMux()
		mux.Handle("/api/v1/metrics", promhttp.HandlerFor(pr.(prometheus.Gatherer), promhttp.HandlerOpts{MaxRequestsInFlight: 1}))
		return httpsrv.Run(ctx,
			http.Server{
				Addr:              config.RPCServerAddress,
				Handler:           mux,
				ReadTimeout:       3 * time.Second,
				ReadHeaderTimeout: time.Second,
				WriteTimeout:      5 * time.Second,
				IdleTimeout:       30 * time.Second,
			})
	})

	return g.Wait()
}

func createHost(ctx context.Context, keys *Keys, cfg *rootNodeConfig) (*network.Peer, error) {
	bootNodes, err := getBootStrapNodes(cfg.BootStrapAddresses)
	if err != nil {
		return nil, fmt.Errorf("boot nodes parameter error: %w", err)
	}
	keyPair, err := keys.getEncryptionKeyPair()
	if err != nil {
		return nil, fmt.Errorf("get key pair failed: %w", err)
	}
	peerConf, err := network.NewPeerConfiguration(cfg.Address, keyPair, bootNodes, nil)
	if err != nil {
		return nil, err
	}

	return network.NewPeer(ctx, peerConf, cfg.Base.observe.Logger(), cfg.Base.observe.PrometheusRegisterer())
}

func verifyKeyPresentInGenesis(peer *network.Peer, rg *genesis.GenesisRootRecord, ver abcrypto.Verifier) error {
	nodeInfo := rg.FindPubKeyById(peer.ID().String())
	if nodeInfo == nil {
		return fmt.Errorf("node id/encode key not found in genesis")
	}
	signPubKeyBytes, err := ver.MarshalPublicKey()
	if err != nil {
		return fmt.Errorf("invalid root node sign key: %w", err)
	}
	// verify that the same public key is present in the genesis file
	if !bytes.Equal(signPubKeyBytes, nodeInfo.SigningPublicKey) {
		return fmt.Errorf("signing key not found in genesis file")
	}
	return nil
}
