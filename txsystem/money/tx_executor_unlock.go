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

func (m *Module) executeUnlockTx(tx *types.TransactionOrder, _ *money.UnlockAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// unlock the unit
	unitID := tx.UnitID()
	action := state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
		newBillData, ok := data.(*money.BillData)
		if !ok {
			return nil, fmt.Errorf("unlock tx: unit %v does not contain bill data", unitID)
		}
		newBillData.Locked = 0
		newBillData.T = exeCtx.CurrentRound()
		newBillData.Counter += 1
		return newBillData, nil
	})
	if err := m.state.Apply(action); err != nil {
		return nil, fmt.Errorf("unlock tx: failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) validateUnlockTx(tx *types.TransactionOrder, attr *money.UnlockAttributes, _ txtypes.ExecutionContext) error {
	unitID := tx.UnitID()
	unit, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("unlock tx: get unit error: %w", err)
	}
	billData, ok := unit.Data().(*money.BillData)
	if !ok {
		return errors.New("unlock tx: invalid unit type")
	}
	if !billData.IsLocked() {
		return ErrBillUnlocked
	}
	if billData.Counter != attr.Counter {
		return ErrInvalidCounter
	}
	return nil
}
