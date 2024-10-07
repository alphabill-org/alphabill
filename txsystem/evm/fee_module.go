package evm

import (
	"crypto"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/templates"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
)

var _ txtypes.Module = (*FeeAccount)(nil)

type (
	FeeAccount struct {
		state         *state.State
		systemID      types.SystemID
		moneySystemID types.SystemID
		trustBase     types.RootTrustBase
		hashAlgorithm crypto.Hash
		feeCalculator FeeCalculator
		execPredicate predicates.PredicateRunner
		log           *slog.Logger
	}

	FeeCalculator func() uint64
)

func FixedFee(fee uint64) FeeCalculator {
	return func() uint64 {
		return fee
	}
}

func newFeeModule(systemIdentifier types.SystemID, options *Options, log *slog.Logger) (*FeeAccount, error) {
	m := &FeeAccount{
		state:         options.state,
		systemID:      systemIdentifier,
		moneySystemID: money.DefaultSystemID,
		trustBase:     options.trustBase,
		hashAlgorithm: options.hashAlgorithm,
		feeCalculator: FixedFee(1),
		log:           log,
	}
	predEng, err := predicates.Dispatcher(templates.New())
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}
	m.execPredicate = predicates.NewPredicateRunner(predEng.Execute)
	return m, nil
}

func (f *FeeAccount) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		fc.TransactionTypeAddFeeCredit:   txtypes.NewTxHandler[fc.AddFeeCreditAttributes, fc.AddFeeCreditAuthProof](f.validateAddFC, f.executeAddFC),
		fc.TransactionTypeCloseFeeCredit: txtypes.NewTxHandler[fc.CloseFeeCreditAttributes, fc.CloseFeeCreditAuthProof](f.validateCloseFC, f.executeCloseFC),
	}
}

func (f *FeeAccount) GenericTransactionValidator() genericTransactionValidator {
	return checkFeeAccountBalance(f.state, f.execPredicate)
}
