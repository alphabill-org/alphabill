package cmd

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/spf13/cobra"
	"path"
	"time"
)

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
	cmd.Flags().StringVar(&config.KeyFile, keyFileCmd, "", "path to root chain key file")
	cmd.Flags().StringVar(&config.GenesisFile, "genesis-file", "", "path to root-genesis.json file (default $ABHOME/rootchain)")
	cmd.Flags().StringVar(&config.Address, "address", "/ip4/127.0.0.1/tcp/53331", "rootchain address in libp2p multiaddress-format")
	cmd.Flags().Uint64Var(&config.T3Timeout, "t3-timeout", 900, "how long root chain nodes wait for message from leader before initiating a new round")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "root chain request buffer capacity")
	err := cmd.MarkFlagRequired(keyFileCmd)
	if err != nil {
		panic(err)
	}
	return cmd
}

// getGenesisFilePath returns genesis file path if provided, otherwise $ABHOME/rootchain/root-genesis.json
// Must be called after $ABHOME is initialized in base command PersistentPreRunE function.
func (c *rootChainConfig) getGenesisFilePath() string {
	if c.GenesisFile != "" {
		return c.GenesisFile
	}
	return path.Join(c.Base.defaultRootGenesisFilePath(), rootGenesisFileName)
}

func defaultRootChainRunFunc(ctx context.Context, config *rootChainConfig) error {
	rk, err := util.ReadJsonFile(config.KeyFile, &rootKey{})
	if err != nil {
		return err
	}
	peer, err := createPeer(config, rk)
	if err != nil {
		return err
	}
	rootGenesis, err := util.ReadJsonFile(config.getGenesisFilePath(), &genesis.RootGenesis{})
	if err != nil {
		return err
	}
	signer, err := rk.toSigner()
	if err != nil {
		return err
	}
	rc, err := rootchain.NewRootChain(peer, rootGenesis, signer,
		rootchain.WithT3Timeout(time.Duration(config.T3Timeout)*time.Millisecond),
		rootchain.WithRequestChCapacity(config.MaxRequests),
	)
	if err != nil {
		return errors.Wrap(err, "rootchain failed to start: %s")
	}
	// use StartAndWait for SIGTERM hook
	return starter.StartAndWait(ctx, "rootchain", func(ctx context.Context) {
		<-ctx.Done()
		rc.Close()
	})
}

func createPeer(config *rootChainConfig, rootKey *rootKey) (*network.Peer, error) {
	keyPair, err := rootKey.toPeerKeyPair()
	if err != nil {
		return nil, err
	}
	conf := &network.PeerConfiguration{
		Address: config.Address,
		KeyPair: keyPair,
	}
	return network.NewPeer(conf)
}
