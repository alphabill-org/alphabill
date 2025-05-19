package cmd

import (
	"fmt"
	"strconv"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
)

type (
	MoneyPartition struct {
		partitionTypeID types.PartitionTypeID
	}
)

func NewMoneyPartition() *MoneyPartition {
	return &MoneyPartition{
		partitionTypeID: moneysdk.PartitionTypeID,
	}
}
func (p *MoneyPartition) PartitionTypeID() types.PartitionTypeID {
	return p.partitionTypeID
}

func (p *MoneyPartition) PartitionTypeIDString() string {
	return "money"
}

func (p *MoneyPartition) DefaultPartitionParams(flags *ShardConfGenerateFlags) map[string]string {
	partitionParams := make(map[string]string, 1)
	alwaysTruePredicate := string(hex.Encode(templates.AlwaysTrueBytes()))

	op := flags.MoneyInitialBillOwnerPredicate
	if op == "" {
		op = alwaysTruePredicate
	}
	partitionParams[moneyInitialBillOwnerPredicate] = op
	partitionParams[moneyInitialBillValue] = strconv.FormatUint(defaultInitialBillValue, 10)
	partitionParams[moneyDCMoneySupplyValue] = strconv.FormatUint(defaultDCMoneySupplyValue, 10)

	return partitionParams
}

func (p *MoneyPartition) NewGenesisState(pdr *types.PartitionDescriptionRecord) (*state.State, error) {
	return newMoneyGenesisState(pdr)
}

func (p *MoneyPartition) CreateTxSystem(flags *ShardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.PathWithDefault(flags.StateFile, StateFileName)
	state, header, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
		return moneysdk.NewUnitData(ui, nodeConf.ShardConf())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load state file: %w", err)
	}

	txs, err := money.NewTxSystem(
		nodeConf.ShardConf(),
		nodeConf.Observability(),
		money.WithHashAlgorithm(nodeConf.HashAlgorithm()),
		money.WithTrustBase(nodeConf.TrustBase()),
		money.WithState(state),
		money.WithExecutedTransactions(header.ExecutedTransactions),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create money tx system: %w", err)
	}
	return txs, err
}
