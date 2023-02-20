package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

func NewMoneyTxSystem(systemIdentifier []byte, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	money, err := NewMoneyModule(systemIdentifier, options)
	if err != nil {
		// TODO test
		return nil, fmt.Errorf("failed to load money module: %w", err)
	}

	feeCredit, err := fc.NewFeeCreditModule(
		fc.WithState(options.state),
		fc.WithHashAlgorithm(options.hashAlgorithm),
		fc.WithTrustBase(options.trustBase),
		fc.WithSystemIdentifier(systemIdentifier),
		fc.WithMoneyTXSystemIdentifier(systemIdentifier),
		fc.WithFeeCalculator(options.feeCalculator),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}

	return txsystem.NewModularTxSystem(
		[]txsystem.Module{money, feeCredit},
		txsystem.WithEndBlockFunctions(money.EndBlockFuncs()),
		txsystem.WithBeginBlockFunctions(money.BeginBlockFuncs()),
		txsystem.WithSystemIdentifier(systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithTrustBase(options.trustBase),
		txsystem.WithState(options.state),
	)
}
