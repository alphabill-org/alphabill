package cmd

import (
	"context"
	"crypto"
	"os"
	"path"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/spf13/cobra"
)

const moneyPartitionDir = "money"

var defaultABMoneySystemIdentifier = []byte{0, 0, 0, 0}

type moneyGenesisConfig struct {
	Base               *baseConfiguration
	SystemIdentifier   []byte
	KeyFile            string
	Output             string
	ForceKeyGeneration bool
	InitialBillValue   uint64 `validate:"gte=0"`
	DCMoneySupplyValue uint64 `validate:"gte=0"`
}

// newMoneyGenesisCmd creates a new cobra command for the alphabill money partition genesis.
func newMoneyGenesisCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &moneyGenesisConfig{Base: baseConfig}
	var cmd = &cobra.Command{
		Use:   "money-genesis",
		Short: "Generates a genesis file for the Alphabill Money partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return abMoneyGenesisRunFun(ctx, config)
		},
	}

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", defaultABMoneySystemIdentifier, "system identifier in HEX format")
	cmd.Flags().BoolVarP(&config.ForceKeyGeneration, "force-key-gen", "f", false, "generates new keys for the node if the key-file does not exist")
	cmd.Flags().StringVarP(&config.KeyFile, keyFileCmd, "k", "", "path to the key file (default: $AB_HOME/money/keys.json). If key file does not exist and flag -f is present then new keys are generated.")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/money/node-genesis.json)")
	cmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value")
	cmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	return cmd
}

func abMoneyGenesisRunFun(_ context.Context, config *moneyGenesisConfig) error {
	moneyPartitionHomePath := path.Join(config.Base.HomeDir, moneyPartitionDir)
	if !util.FileExists(moneyPartitionHomePath) {
		err := os.MkdirAll(moneyPartitionHomePath, 0700) // -rwe------
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(moneyPartitionHomePath)
	if util.FileExists(nodeGenesisFile) {
		return errors.Errorf("node genesis %s exists", nodeGenesisFile)
	}

	keys, err := LoadKeys(config.getKeyFileLocation(), config.ForceKeyGeneration)
	if err != nil {
		return errors.Wrapf(err, "failed to load keys %v", config.getKeyFileLocation())
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	encryptionPublicKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}

	ib := &money.InitialBill{
		ID:    uint256.NewInt(defaultInitialBillId),
		Value: config.InitialBillValue,
		Owner: script.PredicateAlwaysTrue(),
	}

	txSystem, err := money.NewMoneyTxSystem(
		crypto.SHA256,
		ib,
		config.DCMoneySupplyValue,
		money.SchemeOpts.SystemIdentifier(config.SystemIdentifier),
	)
	nodeGenesis, err := partition.NewNodeGenesis(
		txSystem,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithSystemIdentifier(config.SystemIdentifier),
	)
	if err != nil {
		return err
	}
	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *moneyGenesisConfig) getKeyFileLocation() string {
	if c.KeyFile != "" {
		return c.KeyFile
	}
	return path.Join(c.Base.HomeDir, moneyPartitionDir, keysFileName)
}

func (c *moneyGenesisConfig) getNodeGenesisFileLocation(home string) string {
	if c.Output != "" {
		return c.Output
	}
	return path.Join(home, vdGenesisFileName)
}
