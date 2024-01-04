package cmd

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	moneyGenesisFileName      = "node-genesis.json"
	moneyGenesisStateFileName = "node-genesis-state.cbor"
	moneyPartitionDir         = "money"
	defaultInitialBillValue   = 1000000000000000000
	defaultDCMoneySupplyValue = 1000000000000000000
	defaultT2Timeout          = 2500
)

var (
	defaultInitialBillID    = money.NewBillID(nil, []byte{1})
	defaultInitialBillOwner = templates.AlwaysTrueBytes()

	defaultMoneySDR = &genesis.SystemDescriptionRecord{
		SystemIdentifier: money.DefaultSystemIdentifier,
		T2Timeout:        defaultT2Timeout,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         money.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}
	zeroHash = make([]byte, crypto.SHA256.Size())
)

type moneyGenesisConfig struct {
	Base               *baseConfiguration
	SystemIdentifier   types.SystemID
	Keys               *keysConfig
	Output             string
	OutputState        string
	InitialBillID      types.UnitID
	InitialBillValue   uint64 `validate:"gte=0"`
	InitialBillOwner   predicates.PredicateBytes
	DCMoneySupplyValue uint64   `validate:"gte=0"`
	T2Timeout          uint32   `validate:"gte=0"`
	SDRFiles           []string // system description record files
}

// newMoneyGenesisCmd creates a new cobra command for the alphabill money partition genesis.
func newMoneyGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	sysIDbytes := make([]byte, types.SystemIdentifierLength)
	config := &moneyGenesisConfig{
		Base:             baseConfig,
		Keys:             NewKeysConf(baseConfig, moneyPartitionDir),
		InitialBillID:    defaultInitialBillID,
		InitialBillOwner: defaultInitialBillOwner,
	}
	var cmd = &cobra.Command{
		Use:   "money-genesis",
		Short: "Generates a genesis file for the Alphabill Money partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			config.SystemIdentifier, err = types.BytesToSystemID(sysIDbytes)
			if err != nil {
				return fmt.Errorf("partition ID: %w", err)
			}
			return abMoneyGenesisRunFun(cmd.Context(), config)
		},
	}

	cmd.Flags().BytesHexVarP(&sysIDbytes, "system-identifier", "s", money.DefaultSystemIdentifier.Bytes(), "system identifier in HEX format")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/money/node-genesis.json)")
	cmd.Flags().StringVarP(&config.OutputState, "output-state", "", "", "path to the output genesis state file (default: $AB_HOME/money/node-genesis-state.cbor)")
	cmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value")
	cmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	cmd.Flags().StringSliceVarP(&config.SDRFiles, "system-description-record-files", "c", nil, "path to SDR files (one for each partition, including money partition itself; defaults to single money partition only SDR)")
	config.Keys.addCmdFlags(cmd)
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

	if err := writeStateFile(nodeGenesisStateFile, genesisState, config.SystemIdentifier); err != nil {
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
		SystemDescriptionRecords: sdrs,
	}
	res, err := cbor.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal money partition params: %w", err)
	}
	return res, nil
}

func (c *moneyGenesisConfig) getSDRs() ([]*genesis.SystemDescriptionRecord, error) {
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
	err := s.Apply(state.AddUnit(config.InitialBillID, config.InitialBillOwner, &money.BillData{
		V:        config.InitialBillValue,
		T:        0,
		Backlink: nil,
	}))
	if err == nil {
		err = s.AddUnitLog(config.InitialBillID, zeroHash)
	}
	return err
}

func addInitialDustCollectorMoneySupply(s *state.State, config *moneyGenesisConfig) error {
	err := s.Apply(state.AddUnit(money.DustCollectorMoneySupplyID, money.DustCollectorPredicate, &money.BillData{
		V:        config.DCMoneySupplyValue,
		T:        0,
		Backlink: nil,
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
		feeCreditBill := sdr.FeeCreditBill
		if feeCreditBill == nil {
			return fmt.Errorf("fee credit bill is nil in system description record")
		}
		if bytes.Equal(feeCreditBill.UnitId, money.DustCollectorMoneySupplyID) || bytes.Equal(feeCreditBill.UnitId, config.InitialBillID) {
			return fmt.Errorf("fee credit bill ID may not be equal to DC money supply ID or initial bill ID")
		}

		err := s.Apply(state.AddUnit(feeCreditBill.UnitId, feeCreditBill.OwnerPredicate, &money.BillData{
			V:        0,
			T:        0,
			Backlink: nil,
		}))
		if err != nil {
			return err
		}
		if err := s.AddUnitLog(feeCreditBill.UnitId, zeroHash); err != nil {
			return err
		}
	}
	return nil
}

func writeStateFile(path string, s *state.State, systemID types.SystemID) error {
	stateFile, err := os.Create(filepath.Clean(path))
	if err != nil {
		return err
	}
	return s.Serialize(stateFile, &state.Header{
		SystemIdentifier: systemID,
	}, false)
}
