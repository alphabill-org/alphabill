package txsystem

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
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
		if err := pr(stateLock.ExecutionPredicate, p.Proof, tx, exeCtx.WithExArg(tx.StateLockProofSigBytes)); err != nil {
			return fmt.Errorf("state lock's execution predicate failed: %w", err)
		}
	case StateUnlockRollback:
		if err := pr(stateLock.RollbackPredicate, p.Proof, tx, exeCtx.WithExArg(tx.StateLockProofSigBytes)); err != nil {
			return fmt.Errorf("state lock's rollback predicate failed: %w", err)
		}
	default:
		return errors.New("invalid state unlock proof kind")
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

// handleUnlockUnitState - tries to unlock state locked units.
// Returns no error if target units were successfully unlocked or there was nothing to unlock.
// Returns error if any target unit could not be unlocked (e.g. either predicate fails or invalid input is provided).
func (m *GenericTxSystem) handleUnlockUnitState(tx *types.TransactionOrder, unitID types.UnitID, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	txOnHold, err := m.parseTxOnHold(unitID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse txOnHold: %w", err)
	}
	return m.handleTxOnHold(tx, txOnHold, exeCtx)
}

func (m *GenericTxSystem) handleTxOnHold(tx *types.TransactionOrder, txOnHold *types.TransactionOrder, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	if txOnHold == nil {
		return nil, nil // if no tx on hold then nothing to handle
	}
	// need to unlock (or rollback the lock). Fail the tx if no unlock proof is provided
	proof, err := stateUnlockProofFromTx(tx)
	if err != nil {
		return nil, fmt.Errorf("unlock proof error: %w", err)
	}
	// The following line assumes that the pending transaction is valid and has a Payload
	// this will crash if not, a separate method to return state lock or nil would be better
	if err = proof.check(m.pr, tx, txOnHold.StateLock, exeCtx); err != nil {
		return nil, fmt.Errorf("unlock error: %w", err)
	}
	// execute the tx that was "on hold"
	sm, err := m.executeLockedTx(proof, txOnHold, exeCtx)
	if err != nil {
		return nil, err
	}

	// sanity check, verify that TxHandler returned at least one target unit
	if sm == nil || len(sm.TargetUnits) == 0 {
		return nil, errors.New("failed to execute txOnHold: TxHandler returned nil server metadata or no target units")
	}

	// unlock the existing target units
	for _, targetUnit := range sm.TargetUnits {
		if err = m.state.Apply(state.RemoveStateLock(targetUnit)); err != nil {
			if errors.Is(err, avl.ErrNotFound) {
				m.log.Debug("not removing state lock, unit does not exist", logger.UnitID(targetUnit))
				continue
			}
			if errors.Is(err, state.ErrUnitAlreadyUnlocked) {
				m.log.Debug("not removing state lock, unit is already unlocked", logger.UnitID(targetUnit))
				continue
			}
			return nil, fmt.Errorf("failed to release state lock for unit %s: %w", targetUnit, err)
		}
		m.log.Debug("unit state lock removed", logger.UnitID(targetUnit))
	}
	return sm, nil
}

func (m *GenericTxSystem) parseTxOnHold(unitID types.UnitID) (*types.TransactionOrder, error) {
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			// no unit, no state lock, duh
			return nil, nil
		}
		return nil, fmt.Errorf("getting unit: %w", err)
	}
	unit, err := state.ToUnitV1(u)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unit to version 1: %w", err)
	}
	// if unit is not locked, then this method is done - nothing to parse
	if !unit.IsStateLocked() {
		return nil, nil
	}
	// unit has a state lock, any transaction with locked unit must first unlock
	m.log.Debug("unit has a state lock", logger.UnitID(unitID))
	txOnHold := &types.TransactionOrder{Version: 1}
	if err = types.Cbor.Unmarshal(unit.StateLockTx(), txOnHold); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state lock transaction: %w", err)
	}
	return txOnHold, nil
}

func (m *GenericTxSystem) executeLockedTx(proof *StateUnlockProof, txOnHold *types.TransactionOrder, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	switch proof.Kind {
	case StateUnlockExecute:
		exeCtx.SetExecutionType(txtypes.ExecutionTypeCommit)
		sm, err := m.handlers.Execute(txOnHold, exeCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to execute transaction that was on hold: %w", err)
		}
		m.log.Debug("transaction on hold successfully executed", logger.UnitID(txOnHold.GetUnitID()), logger.Data(txOnHold))
		return sm, nil
	case StateUnlockRollback:
		exeCtx.SetExecutionType(txtypes.ExecutionTypeRollback)
		_, _, targetUnits, err := m.handlers.UnmarshalTx(txOnHold, exeCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal transaction that was on hold for rollback: %w", err)
		}
		for _, targetUnit := range targetUnits {
			u, err := m.state.GetUnit(targetUnit, false)
			if err != nil {
				return nil, fmt.Errorf("failed to find rollback tx target unit %s: %w", targetUnit, err)
			}
			unitV1, err := state.ToUnitV1(u)
			if err != nil {
				return nil, fmt.Errorf("failed to convert unit to version 1: %w", err)
			}
			if unitV1.IsDummy() {
				if err = m.state.Apply(state.MarkForDeletion(targetUnit, m.currentRoundNumber+1)); err != nil {
					return nil, fmt.Errorf("failed to mark rollack tx dummy unit for deletion %s: %w", targetUnit, err)
				}
				m.log.Debug("unit marked for deletion", logger.UnitID(targetUnit))
			}
		}
		m.log.Debug("transaction on hold successfully rolled back", logger.UnitID(txOnHold.GetUnitID()), logger.Data(txOnHold))
		return &types.ServerMetadata{TargetUnits: targetUnits, SuccessIndicator: types.TxStatusSuccessful}, nil
	default:
		return nil, errors.New("invalid state unlock proof kind")
	}
}

// executeLockUnitState - validates lock predicate and locks the state of a unit
func (m *GenericTxSystem) executeLockUnitState(tx *types.TransactionOrder, txBytes []byte, targetUnits []types.UnitID, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// transaction contains lock and execution predicate - lock unit
	if err := tx.StateLock.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid state lock parameter: %w", err)
	}
	// for each target unit add dummy unit and lock it or lock existing unit
	for _, targetUnit := range targetUnits {
		if err := m.state.Apply(state.AddDummyUnit(targetUnit)); err != nil && !errors.Is(err, avl.ErrAlreadyExists) {
			return nil, fmt.Errorf("failed to add dummy unit %s: %w", targetUnit, err)
		}
		if err := m.state.Apply(state.SetStateLock(targetUnit, txBytes)); err != nil {
			return nil, fmt.Errorf("state lock: failed to lock the state: %w", err)
		}
		m.log.Debug("unit locked", logger.UnitID(targetUnit), logger.Data(tx))
	}
	// TODO should charge fees for each dummy unit?
	return &types.ServerMetadata{ActualFee: 1, TargetUnits: targetUnits, SuccessIndicator: types.TxStatusSuccessful}, nil
}
