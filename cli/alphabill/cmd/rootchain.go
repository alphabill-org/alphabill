package cmd

import (
	"context"
	"path"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async/future"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/spf13/cobra"
)

const defaultSendCertificateTimeout = 300 * time.Millisecond

type rootChainConfig struct {
	Base *baseConfiguration

	// path to root chain key file
	KeyFile string

	// path to root-genesis.json file
	GenesisFile string

	// rootchain node address in libp2p multiaddress format
	Address string

	// how long root chain nodes wait for message from Leader before initiating a view change
	T3Timeout uint64

	// root chain request channel capacity
	MaxRequests uint
}

// newRootChainCmd creates a new cobra command for root chain
func newRootChainCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &rootChainConfig{
		Base: baseConfig,
	}
	var cmd = &cobra.Command{
		Use:   "root",
		Short: "Starts a root chain node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return defaultRootChainRunFunc(ctx, config)
		},
	}
	cmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", "path to root chain key file")
	cmd.Flags().StringVarP(&config.GenesisFile, "genesis-file", "g", "", "path to root-genesis.json file (default $AB_HOME/rootchain)")
	cmd.Flags().StringVar(&config.Address, "address", "/ip4/127.0.0.1/tcp/26662", "rootchain address in libp2p multiaddress-format")
	cmd.Flags().Uint64Var(&config.T3Timeout, "t3-timeout", 900, "how long root chain nodes wait for message from leader before initiating a new round")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "root chain request buffer capacity")
	err := cmd.MarkFlagRequired(keyFileCmdFlag)
	if err != nil {
		panic(err)
	}
	return cmd
}

// getGenesisFilePath returns genesis file path if provided, otherwise $AB_HOME/rootchain/root-genesis.json
// Must be called after $AB_HOME is initialized in base command PersistentPreRunE function.
func (c *rootChainConfig) getGenesisFilePath() string {
	if c.GenesisFile != "" {
		return c.GenesisFile
	}
	return path.Join(c.Base.defaultRootGenesisDir(), rootGenesisFileName)
}

func defaultRootChainRunFunc(ctx context.Context, config *rootChainConfig) error {
	rootGenesis, err := util.ReadJsonFile(config.getGenesisFilePath(), &genesis.RootGenesis{})
	if err != nil {
		return errors.Wrapf(err, "failed to open root genesis file %s", config.getGenesisFilePath())
	}
	rk, err := LoadKeys(config.KeyFile, false, false)
	if err != nil {
		return errors.Wrapf(err, "failed to read keys %s", config.KeyFile)
	}
	peer, err := createPeer(config, rk.EncryptionPrivateKey)
	if err != nil {
		return errors.Wrap(err, "peer creation failed")
	}

	net, err := network.NewLibP2PRootChainNetwork(peer, config.MaxRequests, defaultSendCertificateTimeout)
	if err != nil {
		return nil
	}
	rc, err := rootchain.NewRootChain(
		peer,
		rootGenesis,
		rk.SigningPrivateKey,

		net,

		rootchain.WithT3Timeout(time.Duration(config.T3Timeout)*time.Millisecond),
		rootchain.WithRequestChCapacity(config.MaxRequests),
	)
	if err != nil {
		return errors.Wrapf(err, "rootchain failed to start: %v", err)
	}
	// use StartAndWait for SIGTERM hook
	return starter.StartAndWait(ctx, "rootchain", func(ctx context.Context) {
		async.MakeWorker("rootchain shutdown hook", func(ctx context.Context) future.Value {
			<-ctx.Done()
			rc.Close()
			return nil
		}).Start(ctx)
	})
}

func createPeer(config *rootChainConfig, encPrivate crypto.PrivKey) (*network.Peer, error) {
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
		Address: config.Address,
		KeyPair: keyPair,
	}
	return network.NewPeer(conf)
}
