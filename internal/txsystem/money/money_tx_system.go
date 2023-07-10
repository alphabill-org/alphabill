package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

func NewTxSystem(opts ...Option) (*txsystem.GenericTxSystem, error) {
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
		fc.WithMoneyTXSystemIdentifier(options.systemIdentifier),
		fc.WithFeeCalculator(options.feeCalculator),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}

	return txsystem.NewGenericTxSystem(
		[]txsystem.Module{money, feeCreditModule},
		txsystem.WithSystemGeneratedTxs(map[string]bool{PayloadTypePruneDC: true}),
		txsystem.WithEndBlockFunctions(money.EndBlockFuncs()),
		txsystem.WithBeginBlockFunctions(money.BeginBlockFuncs()),
		txsystem.WithSystemIdentifier(options.systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
