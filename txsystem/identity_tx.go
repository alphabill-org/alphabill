package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

var _ Module = (*IdentityModule)(nil)

const TxIdentity = "identity"

type IdentityModule struct {
	txExecutor TransactionExecutor
	state      *state.State
	pr         predicates.PredicateRunner
}

type IdentityAttributes struct {
	_     struct{} `cbor:",toarray"`
	Nonce []byte
}

func NewIdentityModule(txExecutor TransactionExecutor, state *state.State) Module {
	engines, err := predicates.Dispatcher(templates.New())
	if err != nil {
		panic(fmt.Errorf("creating predicate executor: %w", err))
	}
	pr := predicates.NewPredicateRunner(engines.Execute, state)
	return &IdentityModule{txExecutor: txExecutor, state: state, pr: pr}
}

func (i *IdentityModule) TxExecutors() map[string]ExecuteFunc {
	return map[string]ExecuteFunc{
		TxIdentity: i.handleIdentityTx().ExecuteFunc(),
	}
}

func (i *IdentityModule) handleIdentityTx() GenericExecuteFunc[IdentityAttributes] {
	return func(tx *types.TransactionOrder, attr *IdentityAttributes, ctx *TxExecutionContext) (*types.ServerMetadata, error) {
		if err := i.validateIdentityTx(tx, ctx); err != nil {
			return nil, fmt.Errorf("invalid identity tx: %w", err)
		}

		return &types.ServerMetadata{ActualFee: 1, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (i *IdentityModule) validateIdentityTx(tx *types.TransactionOrder, ctx *TxExecutionContext) error {
	unitID := tx.UnitID()
	u, err := i.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("identity tx: unable to fetch the unit: %w", err)
	}

	// depending on whether the unit has the state lock or not, the order of the checks is different
	// that is, if the lock is present, bearer check must be performed only after the unit is unlocked, yielding new state
	if u.IsStateLocked() {
		return fmt.Errorf("identity tx: unit is state locked")
	} else if ctx.StateLockReleased {
		// this is the transaction that was "on hold" due to the state lock
		// do nothing, the state lock has been released
	} else {
		// state not locked, check the bearer
		if err := i.verifyUnitOwnerProof(tx, u.Bearer()); err != nil {
			return fmt.Errorf("identity tx: %w", err)
		}

		// check if state has to be locked
		if tx.Payload.StateLock != nil && len(tx.Payload.StateLock.ExecutionPredicate) != 0 {
			// check if it evaluates to true without any input
			err := i.pr(tx.Payload.StateLock.ExecutionPredicate, nil, tx)
			if err != nil {
				// ignore 'err' as we are only interested if the predicate evaluates to true or not
				txBytes, err := cbor.Marshal(tx)
				if err != nil {
					return fmt.Errorf("state lock: failed to marshal tx: %w", err)
				}
				// lock the state
				action := state.SetStateLock(unitID, txBytes)
				if err := i.state.Apply(action); err != nil {
					return fmt.Errorf("state lock: failed to lock the state: %w", err)
				}
			}
		}
	}
	return nil
}

func (i *IdentityModule) verifyUnitOwnerProof(tx *types.TransactionOrder, bearer types.PredicateBytes) error {
	if err := i.pr(bearer, tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x]",
			err, tx.OwnerProof, bearer)
	}

	return nil
}
