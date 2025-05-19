package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill/state"
	abmoney "github.com/alphabill-org/alphabill/txsystem/money"
)

const StateFileName = "state.cbor"

var moneyPartitionInitialBillID = append(make(types.UnitID, 31), 1, money.BillUnitType)

type (
	shardConfGenesisFlags struct {
		*baseFlags
		shardConfFlags
	}
)

func shardConfGenesisCmd(baseFlags *baseFlags) *cobra.Command {
	flags := &shardConfGenesisFlags{baseFlags: baseFlags}
	var cmd = &cobra.Command{
		Use:   "genesis",
		Short: "Generate genesis state from a shard configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return shardConfGenesis(flags)
		},
	}
	flags.addShardConfFlags(cmd)
	return cmd
}

func shardConfGenesis(flags *shardConfGenesisFlags) error {
	if err := os.MkdirAll(flags.HomeDir, 0700); err != nil { // -rwx------
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	shardConf, err := flags.loadShardConf(flags.baseFlags)
	if err != nil {
		return err
	}
	statePath := flags.PathWithDefault("", StateFileName)
	if util.FileExists(statePath) {
		return fmt.Errorf("state file %q already exists", statePath)
	}

	state, err := newGenesisState(shardConf, flags)
	if err != nil {
		return err
	}
	if err := writeStateFile(statePath, state); err != nil {
		return fmt.Errorf("failed to write genesis state file: %w", err)
	}
	return nil
}

func newGenesisState(pdr *types.PartitionDescriptionRecord, flags *shardConfGenesisFlags) (*state.State, error) {
	partition, ok := flags.baseFlags.partitions[pdr.PartitionTypeID]
	if !ok {
		return nil, fmt.Errorf("unsupported partition type %d", pdr.PartitionTypeID)
	}
	return partition.NewGenesisState(pdr)
}

func newMoneyGenesisState(pdr *types.PartitionDescriptionRecord) (*state.State, error) {
	params, err := ParseMoneyPartitionParams(pdr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse money partition params: %w", err)
	}

	s := state.NewEmptyState()

	if err := addInitialBill(s, params); err != nil {
		return nil, fmt.Errorf("could not set initial bill: %w", err)
	}

	if err := addInitialDustCollectorMoneySupply(s, params); err != nil {
		return nil, fmt.Errorf("could not set DC money supply: %w", err)
	}

	return s, nil
}

func addInitialBill(s *state.State, params *MoneyPartitionParams) error {
	billData := money.NewBillData(params.InitialBillValue, params.InitialBillOwnerPredicate)
	err := s.Apply(state.AddUnit(moneyPartitionInitialBillID, billData))
	if err == nil {
		err = s.AddUnitLog(moneyPartitionInitialBillID, nil)
	}
	return err
}

func addInitialDustCollectorMoneySupply(s *state.State, params *MoneyPartitionParams) error {
	billData := money.NewBillData(params.DCMoneySupplyValue, abmoney.DustCollectorPredicate)
	err := s.Apply(state.AddUnit(abmoney.DustCollectorMoneySupplyID, billData))
	if err == nil {
		err = s.AddUnitLog(abmoney.DustCollectorMoneySupplyID, nil)
	}
	return err
}

func writeStateFile(path string, s *state.State) error {
	stateFile, err := os.Create(filepath.Clean(path))
	if err != nil {
		return err
	}
	return s.Serialize(stateFile, false, nil)
}
