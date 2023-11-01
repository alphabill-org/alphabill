package cmd

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/monolithic"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

const (
	boltRootChainStoreFileName = "rootchain.db"
	rootPortCmdFlag            = "root-listener"
	rootBootStrapNodesCmdFlag  = "bootnodes"
	defaultNetworkTimeout      = 1 * time.Second
)

type rootNodeConfig struct {
	Base               *baseConfiguration
	KeyFile            string // path to rootchain chain key file
	GenesisFile        string // path to rootchain-genesis.json file
	PartitionListener  string // partition validator node address (libp2p multiaddress format)
	RootListener       string // Root validator node address (libp2p multiaddress format)
	StoragePath        string // path to Bolt storage file
	MaxRequests        uint   // validator partition certification request channel capacity
	BootStrapAddresses string // boot strap addresses (libp2p multiaddress format)
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
			return defaultRootNodeRunFunc(cmd.Context(), config)
		},
	}

	cmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", "path to node validator key file  (default $AB_HOME/rootchain/"+defaultKeysFileName+")")
	cmd.Flags().StringVar(&config.GenesisFile, "genesis-file", "", "path to root-genesis.json file (default $AB_HOME/rootchain/"+rootGenesisFileName+")")
	cmd.Flags().StringVar(&config.StoragePath, "db", "", "persistent store path (default: $AB_HOME/rootchain/)")
	cmd.Flags().StringVar(&config.PartitionListener, "partition-listener", "/ip4/127.0.0.1/tcp/26662", "validator address in libp2p multiaddress-format")
	cmd.Flags().StringVar(&config.RootListener, rootPortCmdFlag, "/ip4/127.0.0.1/tcp/29666", "validator address in libp2p multiaddress-format")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "request buffer capacity")
	cmd.Flags().StringVar(&config.BootStrapAddresses, rootBootStrapNodesCmdFlag, "", "comma separated list of bootstrap root node addresses id@libp2p-multiaddress-format")
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

func (c *rootNodeConfig) getBootStrapNodes() ([]peer.AddrInfo, error) {
	if c.BootStrapAddresses == "" {
		return []peer.AddrInfo{}, nil
	}
	nodeStrings := splitAndTrim(c.BootStrapAddresses)
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

func defaultRootNodeRunFunc(ctx context.Context, config *rootNodeConfig) error {
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
	prtHost, err := createHost(ctx, config.PartitionListener, keys.EncryptionPrivateKey, config.Base.Logger)
	if err != nil {
		return fmt.Errorf("creating partition host: %w", err)
	}
	log := config.Base.Logger.With(logger.NodeID(prtHost.ID()))
	partitionNet, err := network.NewLibP2PRootChainNetwork(prtHost, config.MaxRequests, defaultNetworkTimeout, log)
	if err != nil {
		return fmt.Errorf("partition network initialization failed: %w", err)
	}
	ver, err := keys.SigningPrivateKey.Verifier()
	if err != nil {
		return fmt.Errorf("invalid root node sign key: %w", err)
	}
	if err := verifyKeyPresentInGenesis(prtHost, rootGenesis.Root, ver); err != nil {
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
	var cm consensus.Manager
	if len(rootGenesis.Root.RootValidators) == 1 {
		// use monolithic consensus algorithm
		cm, err = monolithic.NewMonolithicConsensusManager(
			prtHost.ID().String(),
			rootGenesis,
			partitionCfg,
			keys.SigningPrivateKey,
			log,
			consensus.WithStorage(store))
	} else {
		// Initiate Root validator network
		var rootHost *network.Peer
		rootHost, err = createConsensusHost(ctx, keys, rootGenesis.Root.RootValidators, config, log)
		if err != nil {
			return fmt.Errorf("failed to create root node host, %w", err)
		}
		var rootNet *network.LibP2PNetwork
		rootNet, err = network.NewLibP2RootConsensusNetwork(rootHost, config.MaxRequests, defaultNetworkTimeout, log)
		if err != nil {
			return fmt.Errorf("failed initiate root network, %w", err)
		}
		// Create distributed consensus manager function
		cm, err = abdrc.NewDistributedAbConsensusManager(rootHost.ID(),
			rootGenesis,
			partitionCfg,
			rootNet,
			keys.SigningPrivateKey,
			log,
			consensus.WithStorage(store))
	}
	if err != nil {
		return fmt.Errorf("failed initiate consensus manager: %w", err)
	}
	node, err := rootchain.New(
		prtHost,
		partitionNet,
		partitionCfg,
		cm,
		log)
	if err != nil {
		return fmt.Errorf("failed initiate root node: %w", err)
	}
	return node.Run(ctx)
}

func createConsensusHost(ctx context.Context, keys *Keys, rootValidators []*genesis.PublicKeyInfo, cfg *rootNodeConfig, log *slog.Logger) (*network.Peer, error) {
	pair, err := keys.getEncryptionKeyPair()
	if err != nil {
		return nil, err
	}
	// read bootstrap parameter
	bootNodes, err := cfg.getBootStrapNodes()
	if err != nil {
		return nil, fmt.Errorf("boot nodes parameter error: %w", err)
	}
	selfId, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return nil, err
	}
	var persistentPeerIDs = make(peer.IDSlice, len(rootValidators))
	for i, validator := range rootValidators {
		if selfId.String() == validator.NodeIdentifier {
			if !bytes.Equal(pair.PublicKey, validator.EncryptionPublicKey) {
				return nil, fmt.Errorf("invalid encryption key")
			}
			persistentPeerIDs[i] = selfId
			continue
		}
		nodeID, err := network.NodeIDFromPublicKeyBytes(validator.EncryptionPublicKey)
		if err != nil {
			return nil, fmt.Errorf("invalid encryption key: %X", validator.EncryptionPublicKey)
		}
		if nodeID.String() != validator.NodeIdentifier {
			return nil, fmt.Errorf("invalid nodeID/encryption key combination: %s", nodeID)
		}
		persistentPeerIDs[i] = nodeID
	}
	// Sort validators by public encryption key
	sort.Sort(persistentPeerIDs)
	conf := &network.PeerConfiguration{
		Address:        cfg.RootListener,
		KeyPair:        pair,
		BootstrapPeers: bootNodes,
		Validators:     persistentPeerIDs,
	}
	return network.NewPeer(ctx, conf, log)
}

func createHost(ctx context.Context, address string, encPrivate crypto.PrivKey, log *slog.Logger) (*network.Peer, error) {
	privateKeyBytes, err := encPrivate.Raw()
	if err != nil {
		return nil, err
	}
	publicKeyBytes, err := encPrivate.GetPublic().Raw()
	if err != nil {
		return nil, err
	}
	keyPair := &network.PeerKeyPair{
		PublicKey:  publicKeyBytes,
		PrivateKey: privateKeyBytes,
	}
	peerConf, err := network.NewPeerConfiguration(address, keyPair, nil, nil)
	if err != nil {
		return nil, err
	}

	return network.NewPeer(ctx, peerConf, log)
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
