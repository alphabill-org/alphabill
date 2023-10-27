package fc

import (
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/common/crypto"
	txsystem2 "github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/state"
)

var _ txsystem2.Module = &FeeCredit{}

var (
	ErrSystemIdentifierMissing      = errors.New("system identifier is missing")
	ErrMoneySystemIdentifierMissing = errors.New("money transaction system identifier is missing")
	ErrStateIsNil                   = errors.New("state is nil")
	ErrTrustBaseMissing             = errors.New("trust base is missing")
)

type (

	// FeeCredit contains fee credit related functionality.
	FeeCredit struct {
		systemIdentifier        []byte
		moneySystemIdentifier   []byte
		state                   *state.State
		hashAlgorithm           crypto.Hash
		trustBase               map[string]abcrypto.Verifier
		txValidator             *DefaultFeeCreditTxValidator
		feeCalculator           FeeCalculator
		feeCreditRecordUnitType []byte
	}

	FeeCalculator func() uint64
)

func FixedFee(fee uint64) FeeCalculator {
	return func() uint64 {
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

func (f *FeeCredit) TxExecutors() map[string]txsystem2.TxExecutor {
	return map[string]txsystem2.TxExecutor{
		transactions.PayloadTypeAddFeeCredit:    handleAddFeeCreditTx(f),
		transactions.PayloadTypeCloseFeeCredit:  handleCloseFeeCreditTx(f),
		transactions.PayloadTypeLockFeeCredit:   handleLockFeeCreditTx(f),
		transactions.PayloadTypeUnlockFeeCredit: handleUnlockFeeCreditTx(f),
	}
}

func (f *FeeCredit) GenericTransactionValidator() txsystem2.GenericTransactionValidator {
	return checkFeeCreditBalance(f.state, f.feeCalculator)
}

func validConfiguration(m *FeeCredit) error {
	if len(m.systemIdentifier) == 0 {
		return ErrSystemIdentifierMissing
	}
	if len(m.moneySystemIdentifier) == 0 {
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
