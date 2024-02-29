package money

import (
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

func NewTxSystem(log *slog.Logger, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, fmt.Errorf("money tx system default configuration: %w", err)
	}
	for _, option := range opts {
		option(options)
	}

	money, err := NewMoneyModule(options)
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
		fc.WithFeeCreditRecordUnitType(FeeCreditRecordUnitType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}
	return txsystem.NewGenericTxSystem(
		log,
		feeCreditModule.CheckFeeCreditBalance,
		[]txsystem.Module{money, feeCreditModule},
		txsystem.WithEndBlockFunctions(money.EndBlockFuncs()...),
		txsystem.WithBeginBlockFunctions(money.BeginBlockFuncs()...),
		txsystem.WithSystemIdentifier(options.systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
