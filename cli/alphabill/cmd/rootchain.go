package cmd

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/monolithic"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/spf13/cobra"
)

const (
	rootPortCmdFlag       = "root-listener"
	defaultNetworkTimeout = 300 * time.Millisecond
)

type rootNodeConfig struct {
	Base *baseConfiguration

	// path to rootchain chain key file
	KeyFile string

	// path to rootchain-genesis.json file
	GenesisFile string

	// partition validator node address (libp2p multiaddress format)
	PartitionListener string

	// Root validator node address (libp2p multiaddress format)
	RootListener string

	// root node addresses
	Validators map[string]string

	// path to Bolt storage file
	StoragePath string

	// validator partition certification request channel capacity
	MaxRequests uint
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
	cmd.Flags().StringVarP(&config.GenesisFile, "genesis-file", "g", "", "path to root-genesis.json file (default $AB_HOME/rootchain/"+rootGenesisFileName+")")
	cmd.Flags().StringVarP(&config.StoragePath, "db", "f", "", "persistent store path (default: $AB_HOME/rootchain/)")
	cmd.Flags().StringVar(&config.PartitionListener, "partition-listener", "/ip4/127.0.0.1/tcp/26662", "validator address in libp2p multiaddress-format")
	cmd.Flags().StringVar(&config.RootListener, rootPortCmdFlag, "/ip4/127.0.0.1/tcp/29666", "validator address in libp2p multiaddress-format")
	cmd.Flags().MarkHidden(rootPortCmdFlag)
	cmd.Flags().StringToStringVarP(&config.Validators, "peers", "p", nil, "a map of root node identifiers and addresses. must contain all genesis validator addresses")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "request buffer capacity")
	return cmd
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

func defaultRootNodeRunFunc(ctx context.Context, config *rootNodeConfig) error {
	rootGenesis, err := util.ReadJsonFile(config.getGenesisFilePath(), &genesis.RootGenesis{})
	if err != nil {
		return fmt.Errorf("failed to open root node genesis file %s, %w", config.getGenesisFilePath(), err)
	}
	keys, err := LoadKeys(config.getKeyFilePath(), false, false)
	if err != nil {
		return fmt.Errorf("failed to read key file %s, %w", config.KeyFile, err)
	}
	// check if genesis file is valid and exit early if is not
	if err = rootGenesis.Verify(); err != nil {
		return fmt.Errorf("root genesis verification failed, %w", err)
	}
	// Process partition node network
	prtHost, err := createHost(config.PartitionListener, keys.EncryptionPrivateKey)
	if err != nil {
		return fmt.Errorf("partition host error, %w", err)
	}
	partitionNet, err := network.NewLibP2PRootChainNetwork(prtHost, config.MaxRequests, defaultNetworkTimeout)
	if err != nil {
		return fmt.Errorf("partition network initlization failed, %w", err)
	}
	ver, err := keys.SigningPrivateKey.Verifier()
	if err != nil {
		return fmt.Errorf("invalid root node sign key error, %w", err)
	}
	if verifyKeyPresentInGenesis(prtHost, rootGenesis.Root, ver) != nil {
		return fmt.Errorf("error root node key not found in genesis file")
	}
	// Initiate partition store
	partitionCfg, err := partitions.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	if err != nil {
		return fmt.Errorf("failed to extract partition info from genesis file %s, %w", config.getGenesisFilePath(), err)
	}
	// use monolithic consensus algorithm
	cm, err := monolithic.NewMonolithicConsensusManager(
		prtHost.ID().String(),
		rootGenesis,
		partitionCfg,
		keys.SigningPrivateKey,
		consensus.WithPersistentStoragePath(config.getStoragePath()))
	if err != nil {
		return fmt.Errorf("failed initiate monolithic consensus manager: %w", err)
	}
	node, err := rootchain.New(
		prtHost,
		partitionNet,
		partitionCfg,
		cm)
	if err != nil {
		return fmt.Errorf("failed initiate root node: %w", err)
	}
	return node.Start(ctx)
}

func createHost(address string, encPrivate crypto.PrivKey) (*network.Peer, error) {
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
	conf := &network.PeerConfiguration{
		Address: address,
		KeyPair: keyPair,
	}
	return network.NewPeer(conf)
}

func (c *rootNodeConfig) getPeerAddress(identifier string) (string, error) {
	address, f := c.Validators[identifier]
	if !f {
		return "", fmt.Errorf("address for node %v not found", identifier)
	}
	return address, nil
}

func verifyKeyPresentInGenesis(peer *network.Peer, rg *genesis.GenesisRootRecord, ver abcrypto.Verifier) error {
	nodeInfo := rg.FindPubKeyById(peer.ID().String())
	if nodeInfo == nil {
		return fmt.Errorf("invalid root node encode key")
	}
	signPubKeyBytes, err := ver.MarshalPublicKey()
	if err != nil {
		return fmt.Errorf("invalid root node sign key, cannot start")
	}
	// verify that the same public key is present in the genesis file
	if !bytes.Equal(signPubKeyBytes, nodeInfo.SigningPublicKey) {
		return fmt.Errorf("invalid root node sign key, expected %X, got %X", signPubKeyBytes, nodeInfo.SigningPublicKey)
	}
	return nil
}
