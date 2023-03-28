package cmd

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"time"

	"github.com/alphabill-org/alphabill/internal/async"
	"github.com/alphabill-org/alphabill/internal/async/future"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/monolithic"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partitions"
	"github.com/alphabill-org/alphabill/internal/starter"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/spf13/cobra"
)

const defaultNetworkTimeout = 300 * time.Millisecond

type validatorConfig struct {
	Base *baseConfiguration

	// path to rootvalidator chain key file
	KeyFile string

	// path to rootvalidator-genesis.json file
	GenesisFile string

	// partition validator node address (libp2p multiaddress format)
	PartitionListener string

	// Root validator node address (libp2p multiaddress format)
	RootListener string

	// root validator addresses
	Validators map[string]string

	// path to Bolt storage file
	StoragePath string

	// validator partition certification request channel capacity
	MaxRequests uint
}

// newRootNodeCmd creates a new cobra command for root validator chain
func newRootNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &validatorConfig{
		Base: baseConfig,
	}
	var cmd = &cobra.Command{
		Use:   "root",
		Short: "Starts a root validator node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return defaultValidatorRunFunc(cmd.Context(), config)
		},
	}

	cmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", "path to root validator validator key file  (default $AB_HOME/rootchain/"+defaultKeysFileName+")")
	cmd.Flags().StringVarP(&config.GenesisFile, "genesis-file", "g", "", "path to root-genesis.json file (default $AB_HOME/rootchain/"+rootGenesisFileName+")")
	cmd.Flags().StringVarP(&config.StoragePath, "db", "f", "", "persistent store path (default: $AB_HOME/rootchain/)")
	cmd.Flags().StringVar(&config.PartitionListener, "partition-listener", "/ip4/127.0.0.1/tcp/26662", "validator address in libp2p multiaddress-format")
	cmd.Flags().StringVar(&config.RootListener, "root-listener", "/ip4/127.0.0.1/tcp/29666", "validator address in libp2p multiaddress-format")
	cmd.Flags().StringToStringVarP(&config.Validators, "peers", "p", nil, "a map of root validator identifiers and addresses. must contain all genesis validator addresses")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "request buffer capacity")
	return cmd
}

// getGenesisFilePath returns genesis file path if provided, otherwise $AB_HOME/rootchain/root-genesis.json
// Must be called after $AB_HOME is initialized in base command PersistentPreRunE function.
func (c *validatorConfig) getGenesisFilePath() string {
	if c.GenesisFile != "" {
		return c.GenesisFile
	}
	return path.Join(c.Base.defaultRootGenesisDir(), rootGenesisFileName)
}

func (c *validatorConfig) getStoragePath() string {
	if c.StoragePath != "" {
		return c.StoragePath
	}
	return c.Base.defaultRootGenesisDir()
}

func (c *validatorConfig) getKeyFilePath() string {
	if c.KeyFile != "" {
		return c.KeyFile
	}
	return path.Join(c.Base.defaultRootGenesisDir(), defaultKeysFileName)
}

func defaultValidatorRunFunc(ctx context.Context, config *validatorConfig) error {
	rootGenesis, err := util.ReadJsonFile(config.getGenesisFilePath(), &genesis.RootGenesis{})
	if err != nil {
		return fmt.Errorf("failed to open root validator genesis file %s, %w", config.getGenesisFilePath(), err)
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
		return fmt.Errorf("invalid root validator sign key error, %w", err)
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
	cm, err := monolithic.NewMonolithicConsensusManager(prtHost.ID().String(),
		rootGenesis,
		partitionCfg,
		keys.SigningPrivateKey,
		consensus.WithPersistentStoragePath(config.getStoragePath()))
	if err != nil {
		return fmt.Errorf("failed initiate monolithic consensus manager: %w", err)
	}
	node, err := rootvalidator.NewRootValidatorNode(
		prtHost,
		partitionNet,
		partitionCfg,
		cm)
	if err != nil {
		return fmt.Errorf("failed initiate root node: %w", err)
	}
	// use StartAndWait for SIGTERM hook
	return starter.StartAndWait(ctx, "root validator", func(ctx context.Context) {
		async.MakeWorker("root validator shutdown hook", func(ctx context.Context) future.Value {
			<-ctx.Done()
			node.Close()
			return nil
		}).Start(ctx)
	})
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

func (c *validatorConfig) getPeerAddress(identifier string) (string, error) {
	address, f := c.Validators[identifier]
	if !f {
		return "", fmt.Errorf("address for node %v not found", identifier)
	}
	return address, nil
}

func verifyKeyPresentInGenesis(peer *network.Peer, rg *genesis.GenesisRootRecord, ver abcrypto.Verifier) error {
	nodeInfo := rg.FindPubKeyById(peer.ID().String())
	if nodeInfo == nil {
		return fmt.Errorf("invalid root validator encode key")
	}
	signPubKeyBytes, err := ver.MarshalPublicKey()
	if err != nil {
		return fmt.Errorf("invalid root validator sign key, cannot start")
	}
	// verify that the same public key is present in the genesis file
	if !bytes.Equal(signPubKeyBytes, nodeInfo.SigningPublicKey) {
		return fmt.Errorf("invalid root validator sign key, expected %X, got %X", signPubKeyBytes, nodeInfo.SigningPublicKey)
	}
	return nil
}
