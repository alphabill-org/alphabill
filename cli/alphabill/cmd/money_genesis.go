package cmd

import (
	"context"
	"crypto"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	moneyPartitionDir         = "money"
	defaultInitialBillId      = 1
	defaultInitialBillValue   = 1000000
	defaultDCMoneySupplyValue = 1000000
	defaultT2Timeout          = 2500
)

var defaultABMoneySystemIdentifier = []byte{0, 0, 0, 0}

type moneyGenesisConfig struct {
	Base               *baseConfiguration
	SystemIdentifier   []byte
	Keys               *keysConfig
	Output             string
	InitialBillValue   uint64 `validate:"gte=0"`
	DCMoneySupplyValue uint64 `validate:"gte=0"`
	T2Timeout          uint32 `validate:"gte=0"`
}

// newMoneyGenesisCmd creates a new cobra command for the alphabill money partition genesis.
func newMoneyGenesisCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &moneyGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, moneyPartitionDir)}
	var cmd = &cobra.Command{
		Use:   "money-genesis",
		Short: "Generates a genesis file for the Alphabill Money partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return abMoneyGenesisRunFun(ctx, config)
		},
	}

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", defaultABMoneySystemIdentifier, "system identifier in HEX format")
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/money/node-genesis.json)")
	cmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value")
	cmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	return cmd
}

func abMoneyGenesisRunFun(_ context.Context, config *moneyGenesisConfig) error {
	moneyPartitionHomePath := path.Join(config.Base.HomeDir, moneyPartitionDir)
	if !util.FileExists(moneyPartitionHomePath) {
		err := os.MkdirAll(moneyPartitionHomePath, 0700) // -rwx------
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(moneyPartitionHomePath)
	if util.FileExists(nodeGenesisFile) {
		return errors.Errorf("node genesis %s exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return errors.Wrapf(err, "failed to load keys %v", config.Keys.GetKeyFileLocation())
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
	if err != nil {
		return fmt.Errorf("failed to create money tx system: %w", err)
	}

	params, err := config.getPartitionParams()
	if err != nil {
		return err
	}
	nodeGenesis, err := partition.NewNodeGenesis(
		txSystem,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithSystemIdentifier(config.SystemIdentifier),
		partition.WithT2Timeout(config.T2Timeout),
		partition.WithParams(params),
	)
	if err != nil {
		return err
	}
	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *moneyGenesisConfig) getNodeGenesisFileLocation(home string) string {
	if c.Output != "" {
		return c.Output
	}
	return path.Join(home, vdGenesisFileName)
}

func (c *moneyGenesisConfig) getPartitionParams() (*anypb.Any, error) {
	dst := new(anypb.Any)
	src := &genesis.MoneyPartitionParams{
		InitialBillValue:   c.InitialBillValue,
		DcMoneySupplyValue: c.DCMoneySupplyValue,
	}
	err := dst.MarshalFrom(src)
	if err != nil {
		return nil, err
	}
	return dst, nil
}
