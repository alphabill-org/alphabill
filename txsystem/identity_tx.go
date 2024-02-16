package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

var _ Module = (*IdentityModule)(nil)

const TxIdentity = "identity"

type IdentityModule struct {
	txExecutor TransactionExecutor
	state      *state.State
}

type IdentityAttributes struct{}

func NewIdentityModule(txExecutor TransactionExecutor, state *state.State) Module {
	return &IdentityModule{txExecutor: txExecutor, state: state}
}

func (i IdentityModule) TxExecutors() map[string]ExecuteFunc {
	return map[string]ExecuteFunc{
		TxIdentity: handleIdentityTx(i.txExecutor, i.state).ExecuteFunc(),
	}
}

func handleIdentityTx(txExecutor TransactionExecutor, state *state.State) GenericExecuteFunc[IdentityAttributes] {
	return func(tx *types.TransactionOrder, attr *IdentityAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		if err := validateIdentityTx(txExecutor, tx, state); err != nil {
			return nil, fmt.Errorf("invalid identity tx: %w", err)
		}

		return &types.ServerMetadata{ActualFee: 1, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateIdentityTx(txExecutor TransactionExecutor, tx *types.TransactionOrder, s *state.State) error {
	unitID := tx.UnitID()
	u, err := s.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("identity tx: %w", err)
	}

	// depending on whether the unit has the state lock or not, the order of the checks is different
	// that is, if the lock is present, bearer check must be performed only after the unit is unlocked, yielding new state
	if u.IsStateLocked() {
		if err := validateUnitStateLock(txExecutor, tx, s, u); err != nil {
			return fmt.Errorf("identity tx: %w", err)
		}
		// TODO: unit must have a new state after the unlock
	} else if u.IsStateLockReleased() {
		// this is the transaction that was "on hold" due to the state lock
		// do nothing, the state lock has been released
	} else {
		// state not locked, check the bearer
		if err := VerifyUnitOwnerProof(tx, u.Bearer()); err != nil {
			return fmt.Errorf("identity tx: %w", err)
		}

		// check if state has to be locked
		if tx.Payload.StateLock != nil && len(tx.Payload.StateLock.ExecutionPredicate) != 0 {
			// check if it evaluates to true without any input
			err := predicates.RunPredicate(tx.Payload.StateLock.ExecutionPredicate, nil, nil)
			if err != nil {
				// ignore 'err' as we are only interested if the predicate evaluates to true or not
				txBytes, err := cbor.Marshal(tx)
				if err != nil {
					return fmt.Errorf("state lock: failed to marshal tx: %w", err)
				}
				// lock the state
				action := state.SetStateLock(unitID, txBytes)
				if err := s.Apply(action); err != nil {
					return fmt.Errorf("state lock: failed to lock the state: %w", err)
				}
			}
		}
	}
	return nil
}

type StateUnlockProofKind byte

const (
	StateUnlockExecute StateUnlockProofKind = iota
	StateUnlockRollback
)

type StateUnlockProof struct {
	Kind  StateUnlockProofKind
	Proof []byte
}

// check checks if the state unlock proof is valid, gives error if not
func (p *StateUnlockProof) check(tx *types.TransactionOrder, stateLock *types.StateLock) error {
	ctx := &predicates.PredicateContext{
		Input:        p.Proof,
		PayloadBytes: nil, // nothing?
		Tx:           tx,
	}
	switch p.Kind {
	case StateUnlockExecute:
		err := predicates.RunPredicateWithContext(stateLock.ExecutionPredicate, ctx)
		if err != nil {
			return fmt.Errorf("state lock's execution predicate failed: %w", err)
		}
	case StateUnlockRollback:
		if err := predicates.RunPredicateWithContext(stateLock.RollbackPredicate, ctx); err != nil {
			return fmt.Errorf("state lock's rollback predicate failed: %w", err)
		}
	default:
		return fmt.Errorf("invalid state unlock proof kind")
	}
	return nil
}

func StateUnlockProofFromBytes(b []byte) (*StateUnlockProof, error) {
	if len(b) < 1 {
		return nil, fmt.Errorf("invalid state unlock proof: empty")
	}
	kind := StateUnlockProofKind(b[0])
	proof := b[1:]
	return &StateUnlockProof{Kind: kind, Proof: proof}, nil
}

// TODO: make this function reusable for all allowed transactions
func validateUnitStateLock(txExecutor TransactionExecutor, tx *types.TransactionOrder, s *state.State, u *state.Unit) error {
	stateLockTx := u.StateLockTx()
	// check if unit has a state lock
	if len(stateLockTx) > 0 {
		// need to unlock (or rollback the lock). Fail the tx if no unlock proof is provided
		proof, err := StateUnlockProofFromBytes(tx.StateUnlock)
		if err != nil {
			return fmt.Errorf("unit has a state lock, but tx does not have unlock proof")
		}
		txOnHold := &types.TransactionOrder{}
		if err := cbor.Unmarshal(stateLockTx, txOnHold); err != nil {
			return fmt.Errorf("failed to unmarshal state lock tx: %w", err)
		}
		stateLock := txOnHold.Payload.StateLock
		if stateLock == nil {
			return fmt.Errorf("state lock tx has no state lock")
		}

		if err := proof.check(tx, stateLock); err != nil {
			return err
		}

		// proof is ok, release the lock
		if err := s.Apply(state.SetStateLock(tx.UnitID(), nil)); err != nil {
			return fmt.Errorf("failed to release state lock: %w", err)
		}

		// execute the tx that was "on hold"
		if proof.Kind == StateUnlockExecute {
			sm, err := txExecutor.Execute(txOnHold)
			if err != nil {
				return fmt.Errorf("failed to execute tx that was on hold: %w", err)
			}
			_ = sm.GetActualFee() // TODO: propagate the fee?
		}
	}

	return nil
}
