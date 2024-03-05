package fc

import (
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	fct "github.com/alphabill-org/alphabill/txsystem/fc/types"
	"github.com/alphabill-org/alphabill/types"
)

var _ txsystem.Module = (*FeeCredit)(nil)

var (
	ErrSystemIdentifierMissing      = errors.New("system identifier is missing")
	ErrMoneySystemIdentifierMissing = errors.New("money transaction system identifier is missing")
	ErrStateIsNil                   = errors.New("state is nil")
	ErrTrustBaseMissing             = errors.New("trust base is missing")
)

type (
	// FeeCredit contains fee credit related functionality.
	FeeCredit struct {
		systemIdentifier        types.SystemID
		moneySystemIdentifier   types.SystemID
		state                   *state.State
		hashAlgorithm           crypto.Hash
		trustBase               map[string]abcrypto.Verifier
		txValidator             *DefaultFeeCreditTxValidator
		feeCalculator           FeeCalculator
		execPredicate           func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error
		feeCreditRecordUnitType []byte
	}

	FeeCalculator func() fct.Fee
)

func FixedFee(fee fct.Fee) FeeCalculator {
	return func() fct.Fee {
		return fee
	}
}

func NewFeeCreditModule(opts ...Option) (*FeeCredit, error) {
	m := &FeeCredit{
		hashAlgorithm: crypto.SHA256,
		feeCalculator: FixedFee(1),
	}
	for _, o := range opts {
		o(m)
	}
	if m.execPredicate == nil {
		predEng, err := predicates.Dispatcher(templates.New())
		if err != nil {
			return nil, fmt.Errorf("creating predicate executor: %w", err)
		}
		m.execPredicate = predicates.PredicateRunner(predEng.Execute, m.state)
	}
	if err := validConfiguration(m); err != nil {
		return nil, fmt.Errorf("invalid fee credit module configuration: %w", err)
	}
	m.txValidator = NewDefaultFeeCreditTxValidator(
		m.moneySystemIdentifier,
		m.systemIdentifier,
		m.hashAlgorithm,
		m.trustBase,
		m.feeCreditRecordUnitType,
	)
	return m, nil
}

func (f *FeeCredit) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		transactions.PayloadTypeAddFeeCredit:    handleAddFeeCreditTx(f).ExecuteFunc(),
		transactions.PayloadTypeCloseFeeCredit:  handleCloseFeeCreditTx(f).ExecuteFunc(),
		transactions.PayloadTypeLockFeeCredit:   handleLockFeeCreditTx(f).ExecuteFunc(),
		transactions.PayloadTypeUnlockFeeCredit: handleUnlockFeeCreditTx(f).ExecuteFunc(),
	}
}

func validConfiguration(m *FeeCredit) error {
	if m.systemIdentifier == 0 {
		return ErrSystemIdentifierMissing
	}
	if m.moneySystemIdentifier == 0 {
		return ErrMoneySystemIdentifierMissing
	}
	if m.state == nil {
		return ErrStateIsNil
	}
	if len(m.trustBase) == 0 {
		return ErrTrustBaseMissing
	}
	return nil
}
