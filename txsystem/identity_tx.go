package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

var _ Module = (*IdentityModule)(nil)

type IdentityModule struct {
	state *state.State
}

type IdentityAttributes struct{}

func NewIdentityModule(state *state.State) Module {
	return &IdentityModule{state: state}
}

func (i IdentityModule) TxExecutors() map[string]ExecuteFunc {
	return map[string]ExecuteFunc{
		"identity": handleIdentityTx(i.state).ExecuteFunc(),
	}
}

func handleIdentityTx(state *state.State) GenericExecuteFunc[IdentityAttributes] {
	return func(tx *types.TransactionOrder, attr *IdentityAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		if err := validateIdentityTx(tx, state); err != nil {
			return nil, fmt.Errorf("invalid identity tx: %w", err)
		}

		return &types.ServerMetadata{ActualFee: 1, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateIdentityTx(tx *types.TransactionOrder, s *state.State) error {
	unitID := tx.UnitID()
	u, err := s.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("identity tx: %w", err)
	}

	// depending whether the unit has the state lock or not, the order of the checks is different
	// that is, if the lock is present, bearer check must be performed only after the unit is unlocked, yielding new state
	if u.IsStateLocked() {
		if err := validateUnitStateLock(tx, s, u); err != nil {
			return fmt.Errorf("identity tx: %w", err)
		}
		// TODO: unit must have a new state after the unlock
	}
	if err := VerifyUnitOwnerProof(tx, u.Bearer()); err != nil {
		return fmt.Errorf("identity tx: %w", err)
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

func StateUnlockProofFromBytes(b []byte) (*StateUnlockProof, error) {
	if len(b) < 1 {
		return nil, fmt.Errorf("invalid state unlock proof: empty")
	}
	kind := StateUnlockProofKind(b[0])
	proof := b[1:]
	return &StateUnlockProof{Kind: kind, Proof: proof}, nil
}

// TODO: make this function reusable for all allowed transactions
func validateUnitStateLock(tx *types.TransactionOrder, s *state.State, u *state.Unit) error {
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

		// TODO: state lock predicates by the spec must take current tx as an input
		switch proof.Kind {
		case StateUnlockExecute:
			// check if the execution predicate is satisfied
			err := predicates.RunPredicate(stateLock.ExecutionPredicate, nil, nil)
			if err != nil {
				return fmt.Errorf("state lock's execution predicate failed: %w", err)
			}
			// release the lock and execute the tx that was "on hold"
			// TODO
		case StateUnlockRollback:
			// check if the rollback predicate is satisfied
			err := predicates.RunPredicate(stateLock.RollbackPredicate, nil, nil)
			if err != nil {
				return fmt.Errorf("state lock's rollback predicate failed: %w", err)
			}
			// release the lock and discard the tx that was "on hold"
			// TODO
		default:
			return fmt.Errorf("invalid state unlock proof kind")
		}
	}

	if tx.Payload.StateLock == nil || len(tx.Payload.StateLock.ExecutionPredicate) == 0 {
	}

	return nil
}
