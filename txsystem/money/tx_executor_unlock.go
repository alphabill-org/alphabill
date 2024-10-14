package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrBillUnlocked = errors.New("bill is already unlocked")
)

func (m *Module) executeUnlockTx(tx *types.TransactionOrder, _ *money.UnlockAttributes, _ *money.UnlockAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// unlock the unit
	unitID := tx.GetUnitID()
	action := state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
		newBillData, ok := data.(*money.BillData)
		if !ok {
			return nil, fmt.Errorf("unlock transaction: unit %v does not contain bill data", unitID)
		}
		newBillData.Locked = 0
		newBillData.T = exeCtx.CurrentRound()
		newBillData.Counter += 1
		return newBillData, nil
	})
	if err := m.state.Apply(action); err != nil {
		return nil, fmt.Errorf("unlock transaction: failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) validateUnlockTx(tx *types.TransactionOrder, attr *money.UnlockAttributes, authProof *money.UnlockAuthProof, exeCtx txtypes.ExecutionContext) error {
	unitID := tx.GetUnitID()
	unit, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("unlock transaction: get unit error: %w", err)
	}
	billData, ok := unit.Data().(*money.BillData)
	if !ok {
		return errors.New("unlock transaction: invalid unit type")
	}
	if !billData.IsLocked() {
		return ErrBillUnlocked
	}
	if billData.Counter != attr.Counter {
		return ErrInvalidCounter
	}
	if err = m.execPredicate(unit.Owner(), authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	return nil
}
