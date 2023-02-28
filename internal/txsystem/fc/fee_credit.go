package fc

import (
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
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
		moneyTXSystemIdentifier []byte
		state                   *rma.Tree
		hashAlgorithm           crypto.Hash
		trustBase               map[string]abcrypto.Verifier
		logger                  logger.Logger
		txValidator             *DefaultFeeCreditTxValidator
		feeCalculator           FeeCalculator
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
	m.txValidator = NewDefaultFeeCreditTxValidator(m.moneyTXSystemIdentifier, m.systemIdentifier, m.hashAlgorithm, m.trustBase)
	return m, nil
}

func (f *FeeCredit) TxExecutors() []txsystem.TxExecutor {
	return []txsystem.TxExecutor{
		handleAddFeeCreditTx(f),
		handleCloseFeeCreditTx(f),
	}
}

func (f *FeeCredit) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return checkFeeCreditBalance(f.state, f.feeCalculator)
}

func (f *FeeCredit) TxConverter() txsystem.TxConverters {
	return map[string]txsystem.TxConverter{
		transactions.TypeURLAddFeeCreditOrder:   transactions.ConvertAddFeeCredit,
		transactions.TypeURLCloseFeeCreditOrder: transactions.ConvertCloseFeeCredit,
	}
}

func validConfiguration(m *FeeCredit) error {
	if len(m.systemIdentifier) == 0 {
		return ErrSystemIdentifierMissing
	}
	if len(m.moneyTXSystemIdentifier) == 0 {
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
