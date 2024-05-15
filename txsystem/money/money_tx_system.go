package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

func NewTxSystem(observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, fmt.Errorf("money tx system default configuration: %w", err)
	}
	for _, option := range opts {
		option(options)
	}

	moneyModule, err := NewMoneyModule(options)
	if err != nil {
		return nil, fmt.Errorf("failed to load money module: %w", err)
	}
	feeCreditModule, err := fc.NewFeeCreditModule(
		fc.WithState(options.state),
		fc.WithHashAlgorithm(options.hashAlgorithm),
		fc.WithTrustBase(options.trustBase),
		fc.WithSystemIdentifier(options.systemIdentifier),
		fc.WithMoneySystemIdentifier(options.systemIdentifier),
		fc.WithFeeCalculator(options.feeCalculator),
		fc.WithFeeCreditRecordUnitType(money.FeeCreditRecordUnitType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}
	return txsystem.NewGenericTxSystem(
		options.systemIdentifier,
		feeCreditModule.CheckFeeCreditBalance,
		[]txsystem.Module{moneyModule, feeCreditModule},
		observe,
		txsystem.WithEndBlockFunctions(moneyModule.EndBlockFuncs()...),
		txsystem.WithBeginBlockFunctions(moneyModule.BeginBlockFuncs()...),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
