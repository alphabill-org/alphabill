package txsystem

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/types"
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

func (m *GenericTxSystem) validateUnitStateLock(tx *types.TransactionOrder) error {
	unitID := tx.UnitID()
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			// no unit, no state lock, duh
			return nil
		}
		return fmt.Errorf("getting unit: %w", err)
	}
	stateLockTx := u.StateLockTx()
	// check if unit has a state lock
	if len(stateLockTx) > 0 {
		m.log.Debug(fmt.Sprintf("unit %s has a state lock", unitID))
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

		if err := proof.check(m.pr, tx, stateLock); err != nil {
			return err
		}

		// proof is ok, release the lock
		if err := m.state.Apply(state.SetStateLock(unitID, nil)); err != nil {
			return fmt.Errorf("failed to release state lock: %w", err)
		}

		// execute the tx that was "on hold"
		if proof.Kind == StateUnlockExecute {
			sm, err := m.doExecute(txOnHold, true)
			if err != nil {
				return fmt.Errorf("failed to execute tx that was on hold: %w", err)
			}
			_ = sm.GetActualFee() // fees are taken when the tx is put on hold, so we ignore the fee here
		} else {
			// TODO: rollback for a tx that creates new unit must clean up the unit from the state tree
		}
	}

	return nil
}

// LockUnitState locks the state of a unit if the state lock predicate evaluates to false
func LockUnitState(tx *types.TransactionOrder, pr predicates.PredicateRunner, s *state.State) (bool, error) {
	unitID := tx.UnitID()
	u, err := s.GetUnit(unitID, false)
	if err != nil {
		return false, fmt.Errorf("unable to fetch the unit: %w", err)
	}
	if u.IsStateLocked() {
		return false, fmt.Errorf("unit's '%s' is state locked", unitID)
	}
	// check if state has to be locked
	if tx.Payload.StateLock != nil && len(tx.Payload.StateLock.ExecutionPredicate) != 0 {
		// check if it evaluates to true without any input
		err := pr(tx.Payload.StateLock.ExecutionPredicate, nil, tx)
		if err != nil {
			// ignore 'err' as we are only interested if the predicate evaluates to true or not
			txBytes, err := cbor.Marshal(tx)
			if err != nil {
				return false, fmt.Errorf("state lock: failed to marshal tx: %w", err)
			}
			// lock the state
			action := state.SetStateLock(unitID, txBytes)
			if err := s.Apply(action); err != nil {
				return false, fmt.Errorf("state lock: failed to lock the state: %w", err)
			}
			return true, nil
		}
	}
	return false, nil
}
