package fc

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
)

const (
	GeneralTxCostGasUnits = 400
	GasUnitsPerTema       = 1000
)

var _ txtypes.Module = (*FeeCredit)(nil)

var (
	ErrSystemIdentifierMissing      = errors.New("system identifier is missing")
	ErrMoneySystemIdentifierMissing = errors.New("money transaction system identifier is missing")
	ErrStateIsNil                   = errors.New("state is nil")
	ErrTrustBaseIsNil               = errors.New("trust base is nil")
)

type (
	// FeeCredit contains fee credit related functionality.
	FeeCredit struct {
		systemIdentifier        types.SystemID
		moneySystemIdentifier   types.SystemID
		state                   *state.State
		hashAlgorithm           crypto.Hash
		trustBase               types.RootTrustBase
		execPredicate           predicates.PredicateRunner
		feeCreditRecordUnitType []byte
	}
)

func NewFeeCreditModule(opts ...Option) (*FeeCredit, error) {
	m := &FeeCredit{
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
	if err := validConfiguration(m); err != nil {
		return nil, fmt.Errorf("invalid fee credit module configuration: %w", err)
	}
	return m, nil
}

func (f *FeeCredit) CalculateCost(gasUsed uint64) uint64 {
	cost := (gasUsed + GasUnitsPerTema/2) / GasUnitsPerTema
	// all transactions cost at least 1 tema - to be refined
	if cost == 0 {
		cost = 1
	}
	return cost
}

func (f *FeeCredit) BuyGas(maxTxCost uint64) uint64 {
	return maxTxCost * GasUnitsPerTema
}

func (f *FeeCredit) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		fc.PayloadTypeAddFeeCredit:    txtypes.NewTxHandler[fc.AddFeeCreditAttributes](f.validateAddFC, f.executeAddFC),
		fc.PayloadTypeCloseFeeCredit:  txtypes.NewTxHandler[fc.CloseFeeCreditAttributes](f.validateCloseFC, f.executeCloseFC),
		fc.PayloadTypeLockFeeCredit:   txtypes.NewTxHandler[fc.LockFeeCreditAttributes](f.validateLockFC, f.executeLockFC),
		fc.PayloadTypeUnlockFeeCredit: txtypes.NewTxHandler[fc.UnlockFeeCreditAttributes](f.validateUnlockFC, f.executeUnlockFC),
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
	if m.trustBase == nil {
		return ErrTrustBaseIsNil
	}
	return nil
}
