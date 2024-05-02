package fc

import (
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

var _ txsystem.Module = (*FeeCredit)(nil)

var (
	ErrSystemIdentifierMissing      = errors.New("system identifier is missing")
	ErrMoneySystemIdentifierMissing = errors.New("money transaction system identifier is missing")
	ErrStateIsNil                   = errors.New("state is nil")
	ErrTrustBaseMissing             = errors.New("trust base is missing")
	ErrUnitTypeIsNotFCR             = errors.New("invalid unit identifier: type is not fee credit record")
	ErrUnitDataTypeIsNotFCR         = fmt.Errorf("invalid unit type: unit is not fee credit record")
)

type (
	// FeeCredit contains fee credit related functionality.
	FeeCredit struct {
		systemIdentifier        types.SystemID
		moneySystemIdentifier   types.SystemID
		state                   *state.State
		hashAlgorithm           crypto.Hash
		trustBase               map[string]abcrypto.Verifier
		feeCalculator           FeeCalculator
		execPredicate           func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error
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
	if m.execPredicate == nil {
		predEng, err := predicates.Dispatcher(templates.New())
		if err != nil {
			return nil, fmt.Errorf("creating predicate executor: %w", err)
		}
		m.execPredicate = predicates.NewPredicateRunner(predEng.Execute, m.state)
	}
	if err := validConfiguration(m); err != nil {
		return nil, fmt.Errorf("invalid fee credit module configuration: %w", err)
	}
	return m, nil
}

func (f *FeeCredit) TxHandlers() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		fc.PayloadTypeAddFeeCredit:    txsystem.NewTxHandler[fc.AddFeeCreditAttributes](f.validateAddFC, f.executeAddFC),
		fc.PayloadTypeCloseFeeCredit:  txsystem.NewTxHandler[fc.CloseFeeCreditAttributes](f.validateCloseFC, f.executeCloseFC),
		fc.PayloadTypeLockFeeCredit:   txsystem.NewTxHandler[fc.LockFeeCreditAttributes](f.validateLockFC, f.executeLockFC),
		fc.PayloadTypeUnlockFeeCredit: txsystem.NewTxHandler[fc.UnlockFeeCreditAttributes](f.validateUnlockFC, f.executeUnlockFC),
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
