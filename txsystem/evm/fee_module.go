package evm

import (
	"crypto"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

var _ txsystem.Module = (*FeeAccount)(nil)

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
	return &FeeAccount{
		state:         options.state,
		systemID:      systemIdentifier,
		moneySystemID: money.DefaultSystemID,
		trustBase:     options.trustBase,
		hashAlgorithm: options.hashAlgorithm,
		feeCalculator: FixedFee(1),
		execPredicate: predicates.NewPredicateRunner(options.execPredicate, options.state),
		log:           log,
	}, nil
}

func (f *FeeAccount) TxHandlers() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		fc.PayloadTypeAddFeeCredit:   txsystem.NewTxHandler[fc.AddFeeCreditAttributes](f.validateAddFC, f.executeAddFC),
		fc.PayloadTypeCloseFeeCredit: txsystem.NewTxHandler[fc.CloseFeeCreditAttributes](f.validateCloseFC, f.executeCloseFC),
	}
}

func (f *FeeAccount) GenericTransactionValidator() genericTransactionValidator {
	return checkFeeAccountBalance(f.state, f.execPredicate)
}
