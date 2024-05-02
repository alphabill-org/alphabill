package txsystem

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/fxamacker/cbor/v2"
)

type StateUnlockProofKind byte

const (
	StateUnlockRollback StateUnlockProofKind = iota
	StateUnlockExecute
)

type StateUnlockProof struct {
	Kind  StateUnlockProofKind
	Proof []byte
}

// check checks if the state unlock proof is valid, gives error if not
func (p *StateUnlockProof) check(pr predicates.PredicateRunner, tx *types.TransactionOrder, stateLock *types.StateLock) error {
	switch p.Kind {
	case StateUnlockExecute:
		if err := pr(stateLock.ExecutionPredicate, p.Proof, tx); err != nil {
			return fmt.Errorf("state lock's execution predicate failed: %w", err)
		}
	case StateUnlockRollback:
		if err := pr(stateLock.RollbackPredicate, p.Proof, tx); err != nil {
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

func (m *GenericTxSystem) handleUnlockUnitState(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			// no unit, no state lock, duh
			return nil, nil
		}
		return nil, fmt.Errorf("getting unit: %w", err)
	}
	stateLockTx := u.StateLockTx()
	// check if unit has a state lock, any transaction with locked unit must first unlock
	if len(stateLockTx) > 0 {
		m.log.Debug(fmt.Sprintf("unit %s has a state lock", unitID))
		// need to unlock (or rollback the lock). Fail the tx if no unlock proof is provided
		proof, err := StateUnlockProofFromBytes(tx.StateUnlock)
		if err != nil {
			return nil, fmt.Errorf("unit has a state lock, but tx does not have unlock proof")
		}
		txOnHold := &types.TransactionOrder{}
		if err = cbor.Unmarshal(stateLockTx, txOnHold); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state lock tx: %w", err)
		}
		stateLock := txOnHold.Payload.StateLock
		if stateLock == nil {
			return nil, fmt.Errorf("state lock tx has no state lock")
		}

		if err = proof.check(m.pr, tx, stateLock); err != nil {
			return nil, err
		}

		// proof is ok, release the lock
		if err = m.state.Apply(state.ClearStateLock(unitID)); err != nil {
			return nil, fmt.Errorf("failed to release state lock: %w", err)
		}

		// execute the tx that was "on hold"
		if proof.Kind == StateUnlockExecute {
			sm, err := m.handlers.Execute(txOnHold, exeCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to execute tx that was on hold: %w", err)
			}
			return sm, nil
		} else {
			// TODO: rollback for a tx that creates new unit must clean up the unit from the state tree
		}
	}

	return nil, nil
}

// LockUnitState locks the state of a unit if the state lock predicate evaluates to false
func (m *GenericTxSystem) handleLockUnitState(tx *types.TransactionOrder, exeCtx *TxExecutionContext) ([]types.UnitID, error) {
	if !tx.Payload.IsStateLock() {
		return nil, nil
	}
	// transaction contains lock and execution predicate - lock unit
	unitID := tx.UnitID()
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch the unit: %w", err)
	}
	// if there is no unit it is not locked
	if u.IsStateLocked() {
		return nil, fmt.Errorf("unit's '%s' is state locked", unitID)
	}
	// call validate
	if _, err = m.handlers.Validate(tx, exeCtx); err != nil {
		return nil, fmt.Errorf("tx validation error: %w", err)
	}
	// check if state has to be locked
	// check if it evaluates to true without any input
	err = m.pr(tx.Payload.StateLock.ExecutionPredicate, nil, tx)
	// unexpected, predicate run successfully without parameters, this cannot be used for locking
	if err == nil {
		return nil, fmt.Errorf("invalid lock predicate")
	}
	// ignore 'err' as we are only interested if the predicate evaluates to true or not
	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("state lock: failed to marshal tx: %w", err)
	}
	// todo: handle multiple targets
	// lock the state
	action := state.SetStateLock(unitID, txBytes)
	if err := m.state.Apply(action); err != nil {
		return nil, fmt.Errorf("state lock: failed to lock the state: %w", err)
	}
	return []types.UnitID{unitID}, nil
}
