package money

import (
	"fmt"
	"log/slog"

	txsystem2 "github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

func NewTxSystem(log *slog.Logger, opts ...Option) (*txsystem2.GenericTxSystem, error) {
	options := DefaultOptions()
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
	return txsystem2.NewGenericTxSystem(
		log,
		[]txsystem2.Module{money, feeCreditModule},
		txsystem2.WithEndBlockFunctions(money.EndBlockFuncs()...),
		txsystem2.WithBeginBlockFunctions(money.BeginBlockFuncs()...),
		txsystem2.WithSystemIdentifier(options.systemIdentifier),
		txsystem2.WithHashAlgorithm(options.hashAlgorithm),
		txsystem2.WithState(options.state),
	)
}
