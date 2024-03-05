package evm

import (
	"crypto"
	"log/slog"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	fct "github.com/alphabill-org/alphabill/txsystem/fc/types"
	"github.com/alphabill-org/alphabill/types"
)

var _ txsystem.Module = (*FeeAccount)(nil)

type (
	FeeAccount struct {
		state            *state.State
		systemIdentifier types.SystemID
		trustBase        map[string]abcrypto.Verifier
		hashAlgorithm    crypto.Hash
		txValidator      *fc.DefaultFeeCreditTxValidator
		feeCalculator    FeeCalculator
		execPredicate    func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error
		log              *slog.Logger
	}

	FeeCalculator func() fct.Fee
)

func FixedFee(fee fct.Fee) FeeCalculator {
	return func() fct.Fee {
		return fee
	}
}

func newFeeModule(systemIdentifier types.SystemID, options *Options, log *slog.Logger) (*FeeAccount, error) {
	return &FeeAccount{
		state:            options.state,
		systemIdentifier: systemIdentifier,
		trustBase:        options.trustBase,
		hashAlgorithm:    options.hashAlgorithm,
		txValidator:      fc.NewDefaultFeeCreditTxValidator(options.moneyTXSystemIdentifier, systemIdentifier, options.hashAlgorithm, options.trustBase, nil),
		feeCalculator:    FixedFee(1),
		execPredicate:    predicates.PredicateRunner(options.execPredicate, options.state),
		log:              log,
	}, nil
}

func (m FeeAccount) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		//  fee credit transaction handlers (credit transfers and reclaims only!)
		transactions.PayloadTypeAddFeeCredit:   addFeeCreditTx(m.state, m.hashAlgorithm, m.feeCalculator, m.txValidator).ExecuteFunc(),
		transactions.PayloadTypeCloseFeeCredit: closeFeeCreditTx(m.state, m.hashAlgorithm, m.feeCalculator, m.txValidator, m.log).ExecuteFunc(),
	}
}

func (m FeeAccount) GenericTransactionValidator() genericTransactionValidator {
	return checkFeeAccountBalance(m.state, m.execPredicate)
}
