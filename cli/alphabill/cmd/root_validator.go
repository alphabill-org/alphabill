package cmd

import (
	"bytes"
	"context"
	"path"
	"sort"
	"time"

	"github.com/alphabill-org/alphabill/internal/async"
	"github.com/alphabill-org/alphabill/internal/async/future"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator"
	"github.com/alphabill-org/alphabill/internal/starter"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const defaultNetworkTimeout = 300 * time.Millisecond
const defaultRoundTimeout = 1000 * time.Millisecond

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

	// validator partition certification request channel capacity
	MaxRequests uint
}

// newRootValidatorCmd creates a new cobra command for root validator chain
func newRootValidatorCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &validatorConfig{
		Base: baseConfig,
	}
	var cmd = &cobra.Command{
		Use:   "root-validator",
		Short: "Starts a root validator node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return defaultValidatorRunFunc(ctx, config)
		},
	}
	cmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", "path to root validator validator key file")
	cmd.Flags().StringVarP(&config.GenesisFile, "genesis-file", "g", "", "path to root-genesis.json file (default $AB_HOME/rootchain)")
	cmd.Flags().StringVar(&config.PartitionListener, "partition-listener", "/ip4/127.0.0.1/tcp/25666", "validator address in libp2p multiaddress-format")
	cmd.Flags().StringVar(&config.RootListener, "root-listener", "/ip4/127.0.0.1/tcp/29666", "validator address in libp2p multiaddress-format")
	cmd.Flags().StringToStringVarP(&config.Validators, "peers", "p", nil, "a map of root validator identifiers and addresses. must contain all genesis validator addresses")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "request buffer capacity")
	err := cmd.MarkFlagRequired(keyFileCmdFlag)
	if err != nil {
		panic(err)
	}
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

func defaultValidatorRunFunc(ctx context.Context, config *validatorConfig) error {
	rootGenesis, err := util.ReadJsonFile(config.getGenesisFilePath(), &genesis.RootGenesis{})
	if err != nil {
		return errors.Wrapf(err, "failed to open rootvalidator genesis file %s", config.getGenesisFilePath())
	}
	keys, err := LoadKeys(config.KeyFile, false, false)
	if err != nil {
		return errors.Wrapf(err, "failed to read keys %s", config.KeyFile)
	}
	// Initiate Root validator network
	rootHost, err := loadRootNetworkConfiguration(keys, rootGenesis.Root.RootValidators, config)
	if err != nil {
		return errors.Wrapf(err, "failed to create rootvalidator host")
	}
	rootNet, err := network.NewLibP2RootValidatorNetwork(rootHost, config.MaxRequests, defaultNetworkTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed initiate rootvalidator validator network")
	}
	// Process partition node network
	// TODO: Add persistent addresses to all validators, needed according to spec. right now?
	prtHost, err := createHost(config.PartitionListener, keys.EncryptionPrivateKey)
	if err != nil {
		return errors.Wrap(err, "Partition listener creation failed")
	}
	partitionNet, err := network.NewLibP2PRootChainNetwork(prtHost, config.MaxRequests, defaultNetworkTimeout)
	if err != nil {
		return errors.Wrap(err, "Failed to initiate partition network")
	}
	validator, err := rootvalidator.NewRootValidatorNode(
		prtHost,
		rootHost,
		rootGenesis,
		keys.SigningPrivateKey,
		partitionNet,
		rootNet,
	)
	if err != nil {
		return errors.Wrapf(err, "rootchain failed to start: %v", err)
	}
	// use StartAndWait for SIGTERM hook
	return starter.StartAndWait(ctx, "root validator", func(ctx context.Context) {
		async.MakeWorker("root validator shutdown hook", func(ctx context.Context) future.Value {
			<-ctx.Done()
			validator.Close()
			return nil
		}).Start(ctx)
	})
}

func loadRootNetworkConfiguration(keys *Keys, rootValidators []*genesis.PublicKeyInfo, cfg *validatorConfig) (*network.Peer, error) {
	pair, err := keys.getEncryptionKeyPair()
	if err != nil {
		return nil, err
	}
	selfId, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return nil, err
	}
	var persistentPeers = make([]*network.PeerInfo, len(rootValidators))
	for i, validator := range rootValidators {
		if selfId.String() == validator.NodeIdentifier {
			if !bytes.Equal(pair.PublicKey, validator.EncryptionPublicKey) {
				return nil, errors.New("invalid encryption key")
			}
			persistentPeers[i] = &network.PeerInfo{
				Address:   cfg.RootListener,
				PublicKey: validator.EncryptionPublicKey,
			}
			continue
		}

		peerAddress, err := cfg.getPeerAddress(validator.NodeIdentifier)
		if err != nil {
			return nil, err
		}

		persistentPeers[i] = &network.PeerInfo{
			Address:   peerAddress,
			PublicKey: validator.EncryptionPublicKey,
		}
	}
	// Sort validators by address
	sort.Slice(persistentPeers, func(i, j int) bool {
		return string(persistentPeers[i].PublicKey) < string(persistentPeers[j].PublicKey)
	})

	conf := &network.PeerConfiguration{
		Address:         cfg.RootListener,
		KeyPair:         pair,
		PersistentPeers: persistentPeers,
	}
	return network.NewPeer(conf)
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
		return "", errors.Errorf("address for node %v not found.", identifier)
	}
	return address, nil
}
