package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

var (
	ErrBillUnlocked = errors.New("bill is already unlocked")
)

func (m *Module) handleUnlockTx() txsystem.GenericExecuteFunc[money.UnlockAttributes] {
	return func(tx *types.TransactionOrder, attr *money.UnlockAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()
		unit, _ := m.state.GetUnit(unitID, false)
		if unit == nil {
			return nil, fmt.Errorf("unlock tx: unit not found %X", tx.UnitID())
		}
		if err := m.execPredicate(unit.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
			return nil, err
		}
		billData, ok := unit.Data().(*money.BillData)
		if !ok {
			return nil, errors.New("unlock tx: invalid unit type")
		}
		if err := validateUnlockTx(attr, billData); err != nil {
			return nil, fmt.Errorf("unlock tx: validation failed: %w", err)
		}
		// unlock the unit
		action := state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
			newBillData, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unlock tx: unit %v does not contain bill data", unitID)
			}
			newBillData.Locked = 0
			newBillData.T = exeCtx.CurrentBlockNr
			newBillData.Counter += 1
			return newBillData, nil
		})
		if err := m.state.Apply(action); err != nil {
			return nil, fmt.Errorf("unlock tx: failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: m.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func validateUnlockTx(attr *money.UnlockAttributes, bd *money.BillData) error {
	if attr == nil {
		return ErrTxAttrNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if !bd.IsLocked() {
		return ErrBillUnlocked
	}
	if bd.Counter != attr.Counter {
		return ErrInvalidCounter
	}
	return nil
}
