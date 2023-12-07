package cmd

import (
	"context"
	"crypto"
	"fmt"
	"os"
	"path/filepath"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	moneyGenesisFileName      = "node-genesis.json"
	moneyPartitionDir         = "money"
	defaultInitialBillValue   = 1000000000000000000
	defaultDCMoneySupplyValue = 1000000000000000000
	defaultT2Timeout          = 2500
)

var defaultMoneySDR = &genesis.SystemDescriptionRecord{
	SystemIdentifier: money.DefaultSystemIdentifier,
	T2Timeout:        defaultT2Timeout,
	FeeCreditBill: &genesis.FeeCreditBill{
		UnitId:         money.NewBillID(nil, []byte{2}),
		OwnerPredicate: templates.AlwaysTrueBytes(),
	},
}

var defaultInitialBillID = money.NewBillID(nil, []byte{1})

type moneyGenesisConfig struct {
	Base               *baseConfiguration
	SystemIdentifier   []byte
	Keys               *keysConfig
	Output             string
	InitialBillValue   uint64   `validate:"gte=0"`
	DCMoneySupplyValue uint64   `validate:"gte=0"`
	T2Timeout          uint32   `validate:"gte=0"`
	SDRFiles           []string // system description record files
}

// newMoneyGenesisCmd creates a new cobra command for the alphabill money partition genesis.
func newMoneyGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &moneyGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, moneyPartitionDir)}
	var cmd = &cobra.Command{
		Use:   "money-genesis",
		Short: "Generates a genesis file for the Alphabill Money partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return abMoneyGenesisRunFun(cmd.Context(), config)
		},
	}

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", money.DefaultSystemIdentifier, "system identifier in HEX format")
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/money/node-genesis.json)")
	cmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value")
	cmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	cmd.Flags().StringSliceVarP(&config.SDRFiles, "system-description-record-files", "c", nil, "path to SDR files (one for each partition, including money partion itself; defaults to single money partition only SDR)")
	return cmd
}

func abMoneyGenesisRunFun(_ context.Context, config *moneyGenesisConfig) error {
	moneyPartitionHomePath := filepath.Join(config.Base.HomeDir, moneyPartitionDir)
	if !util.FileExists(moneyPartitionHomePath) {
		err := os.MkdirAll(moneyPartitionHomePath, 0700) // -rwx------
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(moneyPartitionHomePath)
	if util.FileExists(nodeGenesisFile) {
		return fmt.Errorf("node genesis %s exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to load keys %v: %w", config.Keys.GetKeyFileLocation(), err)
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
		ID:    defaultInitialBillID,
		Value: config.InitialBillValue,
		Owner: templates.AlwaysTrueBytes(),
	}

	sdrs, err := config.getSDRFiles()
	if err != nil {
		return err
	}
	txSystem, err := money.NewTxSystem(
		config.Base.observe.Logger(),
		money.WithSystemIdentifier(config.SystemIdentifier),
		money.WithHashAlgorithm(crypto.SHA256),
		money.WithInitialBill(ib),
		money.WithSystemDescriptionRecords(sdrs),
		money.WithDCMoneyAmount(config.DCMoneySupplyValue),
		money.WithTrustBase(map[string]abcrypto.Verifier{"genesis": nil}),
	)
	if err != nil {
		return fmt.Errorf("failed to create money transaction system: %w", err)
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
	return filepath.Join(home, moneyGenesisFileName)
}

func (c *moneyGenesisConfig) getPartitionParams() ([]byte, error) {
	sdrFiles, err := c.getSDRFiles()
	if err != nil {
		return nil, err
	}
	src := &genesis.MoneyPartitionParams{
		InitialBillValue:         c.InitialBillValue,
		DcMoneySupplyValue:       c.DCMoneySupplyValue,
		SystemDescriptionRecords: sdrFiles,
	}
	res, err := cbor.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal money partition params: %w", err)
	}
	return res, nil
}

func (c *moneyGenesisConfig) getSDRFiles() ([]*genesis.SystemDescriptionRecord, error) {
	var sdrs []*genesis.SystemDescriptionRecord
	if len(c.SDRFiles) == 0 {
		sdrs = append(sdrs, defaultMoneySDR)
	} else {
		for _, sdrFile := range c.SDRFiles {
			sdr, err := util.ReadJsonFile(sdrFile, &genesis.SystemDescriptionRecord{})
			if err != nil {
				return nil, err
			}
			sdrs = append(sdrs, sdr)
		}
	}
	return sdrs, nil
}
