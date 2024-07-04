package txsystem

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
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
func (p *StateUnlockProof) check(pr predicates.PredicateRunner, tx *types.TransactionOrder, stateLock *types.StateLock, exeCtx txtypes.ExecutionContext) error {
	if stateLock == nil {
		return fmt.Errorf("StateLock is nil")
	}
	switch p.Kind {
	case StateUnlockExecute:
		if err := pr(stateLock.ExecutionPredicate, p.Proof, tx, exeCtx); err != nil {
			return fmt.Errorf("state lock's execution predicate failed: %w", err)
		}
	case StateUnlockRollback:
		if err := pr(stateLock.RollbackPredicate, p.Proof, tx, exeCtx); err != nil {
			return fmt.Errorf("state lock's rollback predicate failed: %w", err)
		}
	default:
		return fmt.Errorf("invalid state unlock proof kind")
	}
	return nil
}

func stateUnlockProofFromTx(tx *types.TransactionOrder) (*StateUnlockProof, error) {
	if len(tx.StateUnlock) < 1 {
		return nil, fmt.Errorf("invalid state unlock proof: empty")
	}
	kind := StateUnlockProofKind(tx.StateUnlock[0])
	proof := tx.StateUnlock[1:]
	return &StateUnlockProof{Kind: kind, Proof: proof}, nil
}

// handleUnlockUnitState - tries to unlock a state locked unit.
// Returns error if unit is locked and could not be unlocked (either predicate fails or none input is provided).
func (m *GenericTxSystem) handleUnlockUnitState(tx *types.TransactionOrder, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// todo: handle multiple target units
	unitID := tx.UnitID()
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			// no unit, no state lock, duh
			return nil, nil
		}
		return nil, fmt.Errorf("getting unit: %w", err)
	}
	// if unit is not locked, then this method is done - nothing to unlock
	if !u.IsStateLocked() {
		return nil, nil
	}
	// unit has a state lock, any transaction with locked unit must first unlock
	m.log.Debug(fmt.Sprintf("unit %s has a state lock", unitID))
	// need to unlock (or rollback the lock). Fail the tx if no unlock proof is provided
	proof, err := stateUnlockProofFromTx(tx)
	if err != nil {
		return nil, fmt.Errorf("unlock proof error: %w", err)
	}
	txOnHold := &types.TransactionOrder{}
	if err = types.Cbor.Unmarshal(u.StateLockTx(), txOnHold); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state lock tx: %w", err)
	}
	// The following line assumes that the pending transaction is valid and has a Payload
	// this will crash if not, a separate method to return state lock or nil would be better
	if err = proof.check(m.pr, tx, txOnHold.Payload.StateLock, exeCtx); err != nil {
		return nil, fmt.Errorf("unlock error: %w", err)
	}
	// proof is ok, release the lock
	if err = m.state.Apply(state.SetStateLock(unitID, nil)); err != nil {
		return nil, fmt.Errorf("failed to release state lock: %w", err)
	}
	// execute the tx that was "on hold"
	if proof.Kind == StateUnlockExecute {
		sm, err := m.handlers.Execute(txOnHold, exeCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to execute tx that was on hold: %w", err)
		}
		return sm, nil
	}
	// TODO: AB-1584 rollback for a tx that creates new unit must clean up the unit from the state tree
	return nil, fmt.Errorf("rollaback not yet implemented")
}

// executeLockUnitState - validates lock predicate and locks the state of a unit
func (m *GenericTxSystem) executeLockUnitState(tx *types.TransactionOrder, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// transaction contains lock and execution predicate - lock unit
	if err := tx.Payload.StateLock.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid state lock parameter: %w", err)
	}
	// todo: add support for multiple targets
	targetUnits := []types.UnitID{tx.UnitID()}
	// ignore 'err' as we are only interested if the predicate evaluates to true or not
	txBytes, err := types.Cbor.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("state lock: failed to marshal tx: %w", err)
	}
	// lock the state
	for _, targetUnit := range targetUnits {
		if err = m.state.Apply(state.SetStateLock(targetUnit, txBytes)); err != nil {
			return nil, fmt.Errorf("state lock: failed to lock the state: %w", err)
		}
		m.log.Debug("unit locked", logger.UnitID(targetUnit), logger.Data(tx), logger.Round(m.CurrentRound()))
	}
	return &types.ServerMetadata{ActualFee: 1, TargetUnits: targetUnits, SuccessIndicator: types.TxStatusSuccessful}, nil
}
