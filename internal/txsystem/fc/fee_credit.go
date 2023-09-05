package fc

import (
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

var _ txsystem.Module = &FeeCredit{}

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
		logger                  logger.Logger
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
		logger:        logger.CreateForPackage(),
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

func (f *FeeCredit) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		transactions.PayloadTypeAddFeeCredit:   handleAddFeeCreditTx(f),
		transactions.PayloadTypeCloseFeeCredit: handleCloseFeeCreditTx(f),
	}
}

func (f *FeeCredit) GenericTransactionValidator() txsystem.GenericTransactionValidator {
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
