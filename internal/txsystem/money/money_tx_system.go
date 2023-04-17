package money

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
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
		return nil, fmt.Errorf("failed to load money module: %w", err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}

	modules := []txsystem.Module{money}

	// disable fee handling if fee is calculated to 0 (used to temporarily disable fee handling, can be removed after all wallets are updated)
	if options.feeCalculator() > 0 {
		feeCreditModule, err := fc.NewFeeCreditModule(
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
		modules = append(modules, feeCreditModule)
	}

	return txsystem.NewGenericTxSystem(
		modules,
		txsystem.WithEndBlockFunctions(money.EndBlockFuncs()),
		txsystem.WithBeginBlockFunctions(money.BeginBlockFuncs()),
		txsystem.WithSystemIdentifier(systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}

func decrFeeCredit(tx txsystem.GenericTransaction, feeCalc fc.FeeCalculator, hashAlgo crypto.Hash) rma.Action {
	// disable fee handling if fee is calculated to 0 (used to temporarily disable fee handling, can be removed after all wallets are updated)
	var decrCreditFunc rma.Action
	if feeCalc() == 0 {
		decrCreditFunc = func(tree *rma.Tree) error {
			return nil
		}
	} else {
		fcrID := tx.ToProtoBuf().GetClientFeeCreditRecordID()
		decrCreditFunc = fc.DecrCredit(fcrID, feeCalc(), tx.Hash(hashAlgo))
	}
	return decrCreditFunc
}
