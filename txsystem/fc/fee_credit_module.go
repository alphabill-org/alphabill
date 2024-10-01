package fc

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

const (
	GeneralTxCostGasUnits = 400
	GasUnitsPerTema       = 1000
)

var _ txtypes.FeeCreditModule = (*FeeCreditModule)(nil)

var (
	ErrSystemIdentifierMissing      = errors.New("system identifier is missing")
	ErrMoneySystemIdentifierMissing = errors.New("money transaction system identifier is missing")
	ErrStateIsNil                   = errors.New("state is nil")
	ErrTrustBaseIsNil               = errors.New("trust base is nil")
)

type (
	// FeeCreditModule contains fee credit related functionality.
	FeeCreditModule struct {
		systemIdentifier        types.SystemID
		moneySystemIdentifier   types.SystemID
		state                   *state.State
		hashAlgorithm           crypto.Hash
		trustBase               types.RootTrustBase
		execPredicate           predicates.PredicateRunner
		feeBalanceValidator     *FeeBalanceValidator
		feeCreditRecordUnitType []byte
	}
)

func NewFeeCreditModule(opts ...Option) (*FeeCreditModule, error) {
	m := &FeeCreditModule{
		hashAlgorithm: crypto.SHA256,
	}
	for _, o := range opts {
		o(m)
	}
	if m.execPredicate == nil {
		predEng, err := predicates.Dispatcher(templates.New())
		if err != nil {
			return nil, fmt.Errorf("creating predicate executor: %w", err)
		}
		m.execPredicate = predicates.NewPredicateRunner(predEng.Execute)
	}
	if m.feeBalanceValidator == nil {
		m.feeBalanceValidator = NewFeeBalanceValidator(m.state, m.execPredicate, m.feeCreditRecordUnitType)
	}
	if err := m.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid fee credit module configuration: %w", err)
	}
	return m, nil
}

func (f *FeeCreditModule) CalculateCost(gasUsed uint64) uint64 {
	cost := (gasUsed + GasUnitsPerTema/2) / GasUnitsPerTema
	// all transactions cost at least 1 tema - to be refined
	if cost == 0 {
		cost = 1
	}
	return cost
}

func (f *FeeCreditModule) BuyGas(maxTxCost uint64) uint64 {
	return maxTxCost * GasUnitsPerTema
}

func (f *FeeCreditModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		fc.TransactionTypeAddFeeCredit:    txtypes.NewTxHandler[fc.AddFeeCreditAttributes, fc.AddFeeCreditAuthProof](f.validateAddFC, f.executeAddFC),
		fc.TransactionTypeCloseFeeCredit:  txtypes.NewTxHandler[fc.CloseFeeCreditAttributes, fc.CloseFeeCreditAuthProof](f.validateCloseFC, f.executeCloseFC),
		fc.TransactionTypeLockFeeCredit:   txtypes.NewTxHandler[fc.LockFeeCreditAttributes, fc.LockFeeCreditAuthProof](f.validateLockFC, f.executeLockFC),
		fc.TransactionTypeUnlockFeeCredit: txtypes.NewTxHandler[fc.UnlockFeeCreditAttributes, fc.UnlockFeeCreditAuthProof](f.validateUnlockFC, f.executeUnlockFC),
	}
}

func (f *FeeCreditModule) IsFeeCreditTx(tx *types.TransactionOrder) bool {
	return fc.IsFeeCreditTx(tx)
}

func (f *FeeCreditModule) IsValid() error {
	if f.systemIdentifier == 0 {
		return ErrSystemIdentifierMissing
	}
	if f.moneySystemIdentifier == 0 {
		return ErrMoneySystemIdentifierMissing
	}
	if f.state == nil {
		return ErrStateIsNil
	}
	if f.trustBase == nil {
		return ErrTrustBaseIsNil
	}
	return nil
}

func (f *FeeCreditModule) IsCredible(exeCtx txtypes.ExecutionContext, tx *types.TransactionOrder) error {
	return f.feeBalanceValidator.IsCredible(exeCtx, tx)
}
