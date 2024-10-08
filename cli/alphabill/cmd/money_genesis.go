package cmd

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/money"
)

const (
	moneyGenesisFileName      = "node-genesis.json"
	moneyGenesisStateFileName = "node-genesis-state.cbor"
	moneyPartitionDir         = "money"
	defaultInitialBillValue   = 1000000000000000000
	defaultDCMoneySupplyValue = 1000000000000000000
)

var (
	defaultInitialBillID             = moneysdk.NewBillID(nil, []byte{1})
	defaultInitialBillOwnerPredicate = templates.AlwaysTrueBytes()

	defaultMoneyPDR = &types.PartitionDescriptionRecord{
		NetworkIdentifier: types.NetworkLocal,
		SystemIdentifier:  moneysdk.DefaultSystemID,
		TypeIdLen:         8,
		UnitIdLen:         256,
		T2Timeout:         2500 * time.Millisecond,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         moneysdk.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}
	zeroHash = make([]byte, crypto.SHA256.Size())
)

type moneyGenesisConfig struct {
	Base                      *baseConfiguration
	Keys                      *keysConfig
	Output                    string
	OutputState               string
	PDRFilename               string
	InitialBillID             types.UnitID
	InitialBillValue          uint64
	InitialBillOwnerPredicate []byte
	DCMoneySupplyValue        uint64
	SDRFiles                  []string // system description record filenames
}

// newMoneyGenesisCmd creates a new cobra command for the alphabill money partition genesis.
func newMoneyGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &moneyGenesisConfig{
		Base:                      baseConfig,
		Keys:                      NewKeysConf(baseConfig, moneyPartitionDir),
		InitialBillID:             defaultInitialBillID,
		InitialBillOwnerPredicate: defaultInitialBillOwnerPredicate,
	}
	var cmd = &cobra.Command{
		Use:   "money-genesis",
		Short: "Generates a genesis file for the Alphabill Money partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return abMoneyGenesisRunFun(cmd.Context(), config)
		},
	}

	cmd.Flags().StringVar(&config.PDRFilename, "partition-description", "", "filename (full path) from where to read the Partition Description Record")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/money/node-genesis.json)")
	cmd.Flags().StringVarP(&config.OutputState, "output-state", "", "", "path to the output genesis state file (default: $AB_HOME/money/node-genesis-state.cbor)")
	cmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value")
	cmd.Flags().BytesHexVar(&config.InitialBillOwnerPredicate, "initial-bill-owner-predicate", defaultInitialBillOwnerPredicate, "the initial bill owner predicate")
	cmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	cmd.Flags().StringSliceVarP(&config.SDRFiles, "system-description-record-files", "c", nil, "path to SDR files (one for each partition, including money partition itself; defaults to single money partition only SDR)")
	config.Keys.addCmdFlags(cmd)
	_ = cmd.MarkFlagRequired("partition-description")
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
		return fmt.Errorf("node genesis file %q already exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	nodeGenesisStateFile := config.getNodeGenesisStateFileLocation(moneyPartitionHomePath)
	if util.FileExists(nodeGenesisStateFile) {
		return fmt.Errorf("node genesis state file %q already exists", nodeGenesisStateFile)
	}

	pdr, err := util.ReadJsonFile(config.PDRFilename, &types.PartitionDescriptionRecord{})
	if err != nil {
		return fmt.Errorf("loading partition description: %w", err)
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

	// An uncommitted state, no UC yet
	genesisState, err := newGenesisState(config)
	if err != nil {
		return err
	}

	params, err := config.getPartitionParams()
	if err != nil {
		return err
	}
	nodeGenesis, err := partition.NewNodeGenesis(
		genesisState,
		*pdr,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithParams(params),
	)
	if err != nil {
		return err
	}

	if err := writeStateFile(nodeGenesisStateFile, genesisState); err != nil {
		return fmt.Errorf("failed to write genesis state file: %w", err)
	}

	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *moneyGenesisConfig) getNodeGenesisFileLocation(home string) string {
	if c.Output != "" {
		return c.Output
	}
	return filepath.Join(home, moneyGenesisFileName)
}

func (c *moneyGenesisConfig) getNodeGenesisStateFileLocation(home string) string {
	if c.OutputState != "" {
		return c.OutputState
	}
	return filepath.Join(home, moneyGenesisStateFileName)
}

func (c *moneyGenesisConfig) getPartitionParams() ([]byte, error) {
	sdrs, err := c.getSDRs()
	if err != nil {
		return nil, err
	}
	src := &genesis.MoneyPartitionParams{
		Partitions: sdrs,
	}
	res, err := types.Cbor.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal money partition params: %w", err)
	}
	return res, nil
}

func (c *moneyGenesisConfig) getSDRs() ([]*types.PartitionDescriptionRecord, error) {
	var sdrs []*types.PartitionDescriptionRecord
	if len(c.SDRFiles) == 0 {
		sdrs = append(sdrs, defaultMoneyPDR)
	} else {
		for _, sdrFile := range c.SDRFiles {
			sdr, err := util.ReadJsonFile(sdrFile, &types.PartitionDescriptionRecord{})
			if err != nil {
				return nil, err
			}
			sdrs = append(sdrs, sdr)
		}
	}
	return sdrs, nil
}

func newGenesisState(config *moneyGenesisConfig) (*state.State, error) {
	s := state.NewEmptyState()

	if err := addInitialBill(s, config); err != nil {
		return nil, fmt.Errorf("could not set initial bill: %w", err)
	}

	if err := addInitialDustCollectorMoneySupply(s, config); err != nil {
		return nil, fmt.Errorf("could not set DC money supply: %w", err)
	}

	if err := addInitialFeeCreditBills(s, config); err != nil {
		return nil, fmt.Errorf("could not set initial fee credits: %w", err)
	}

	return s, nil
}

func addInitialBill(s *state.State, config *moneyGenesisConfig) error {
	err := s.Apply(state.AddUnit(config.InitialBillID, config.InitialBillOwnerPredicate, &moneysdk.BillData{
		V:       config.InitialBillValue,
		T:       0,
		Counter: 0,
	}))
	if err == nil {
		err = s.AddUnitLog(config.InitialBillID, zeroHash)
	}
	return err
}

func addInitialDustCollectorMoneySupply(s *state.State, config *moneyGenesisConfig) error {
	err := s.Apply(state.AddUnit(money.DustCollectorMoneySupplyID, money.DustCollectorPredicate, &moneysdk.BillData{
		V:       config.DCMoneySupplyValue,
		T:       0,
		Counter: 0,
	}))
	if err == nil {
		err = s.AddUnitLog(money.DustCollectorMoneySupplyID, zeroHash)
	}
	return err
}

func addInitialFeeCreditBills(s *state.State, config *moneyGenesisConfig) error {
	sdrs, err := config.getSDRs()
	if err != nil {
		return err
	}

	if len(sdrs) == 0 {
		return fmt.Errorf("undefined system description records")
	}

	for _, sdr := range sdrs {
		fcb := sdr.FeeCreditBill
		if fcb == nil {
			return fmt.Errorf("fee credit bill is nil in system description record")
		}
		if !fcb.UnitID.HasType(moneysdk.BillUnitType) {
			return fmt.Errorf("fee credit bill ID has wrong unit type")
		}
		if bytes.Equal(fcb.UnitID, money.DustCollectorMoneySupplyID) || bytes.Equal(fcb.UnitID, config.InitialBillID) {
			return fmt.Errorf("fee credit bill ID may not be equal to DC money supply ID or initial bill ID")
		}

		err := s.Apply(state.AddUnit(fcb.UnitID, fcb.OwnerPredicate, &moneysdk.BillData{
			V:       0,
			T:       0,
			Counter: 0,
		}))
		if err != nil {
			return err
		}
		if err := s.AddUnitLog(fcb.UnitID, zeroHash); err != nil {
			return err
		}
	}
	return nil
}

func writeStateFile(path string, s *state.State) error {
	stateFile, err := os.Create(filepath.Clean(path))
	if err != nil {
		return err
	}
	return s.Serialize(stateFile, false)
}
