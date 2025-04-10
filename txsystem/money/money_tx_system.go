package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	basetypes "github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func NewTxSystem(shardConf *basetypes.PartitionDescriptionRecord, observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions(observe)
	if err != nil {
		return nil, fmt.Errorf("money transaction system default configuration: %w", err)
	}
	for _, option := range opts {
		option(options)
	}

	moneyModule, err := NewMoneyModule(*shardConf, options)
	if err != nil {
		return nil, fmt.Errorf("failed to load money module: %w", err)
	}
	feeCreditModule, err := fc.NewFeeCreditModule(*shardConf, shardConf.PartitionID, options.state, options.trustBase, observe,
		fc.WithHashAlgorithm(options.hashAlgorithm),
		fc.WithFeeCreditRecordUnitType(money.FeeCreditRecordUnitType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}
	return txsystem.NewGenericTxSystem(
		*shardConf,
		options.trustBase,
		[]txtypes.Module{moneyModule},
		observe,
		txsystem.WithFeeCredits(feeCreditModule),
		txsystem.WithEndBlockFunctions(moneyModule.EndBlockFuncs()...),
		txsystem.WithBeginBlockFunctions(moneyModule.BeginBlockFuncs()...),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
		txsystem.WithExecutedTransactions(options.executedTransactions),
	)
}
