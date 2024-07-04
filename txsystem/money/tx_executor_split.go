package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *Module) executeSplitTx(tx *types.TransactionOrder, attr *money.SplitAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	targetUnitIDs := []types.UnitID{unitID}
	// add new units
	var actions []state.Action
	for i, targetUnit := range attr.TargetUnits {
		newUnitID := money.NewBillID(unitID, tx.HashForNewUnitID(m.hashAlgorithm, util.Uint32ToBytes(uint32(i))))
		targetUnitIDs = append(targetUnitIDs, newUnitID)
		actions = append(actions, state.AddUnit(
			newUnitID,
			targetUnit.OwnerCondition,
			&money.BillData{
				V:       targetUnit.Amount,
				T:       exeCtx.CurrentBlockNumber,
				Counter: 0,
			}))
	}
	// update existing unit
	actions = append(actions, state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			return &money.BillData{
				V:       attr.RemainingValue,
				T:       exeCtx.CurrentBlockNumber,
				Counter: bd.Counter + 1,
			}, nil
		},
	))
	// update state
	if err := m.state.Apply(actions...); err != nil {
		return nil, fmt.Errorf("state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: m.feeCalculator(), TargetUnits: targetUnitIDs, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) validateSplitTx(tx *types.TransactionOrder, attr *money.SplitAttributes, _ *txsystem.TxExecutionContext) error {
	unit, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	if err = validateSplit(unit.Data(), attr); err != nil {
		return fmt.Errorf("split error: %w", err)
	}
	if err = m.execPredicate(unit.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	return nil
}

func validateSplit(data types.UnitData, attr *money.SplitAttributes) error {
	bd, ok := data.(*money.BillData)
	if !ok {
		return errors.New("invalid data type, unit is not of BillData type")
	}
	if bd.IsLocked() {
		return ErrBillLocked
	}
	if bd.Counter != attr.Counter {
		return ErrInvalidCounter
	}
	if len(attr.TargetUnits) == 0 {
		return errors.New("target units are empty")
	}
	var sum uint64
	for i, targetUnit := range attr.TargetUnits {
		if targetUnit == nil {
			return fmt.Errorf("target unit is nil at index %d", i)
		}
		if targetUnit.Amount == 0 {
			return fmt.Errorf("target unit amount is zero at index %d", i)
		}
		if len(targetUnit.OwnerCondition) == 0 {
			return fmt.Errorf("target unit owner condition is empty at index %d", i)
		}
		var err error
		sum, _, err = util.AddUint64(sum, targetUnit.Amount)
		if err != nil {
			return fmt.Errorf("failed to add target unit amounts: %w", err)
		}
	}
	if attr.RemainingValue == 0 {
		return errors.New("remaining value is zero")
	}
	if attr.RemainingValue != bd.V-sum {
		return fmt.Errorf(
			"the sum of the values to be transferred plus the remaining value must equal the value of the bill"+
				"; sum=%d remainingValue=%d billValue=%d", sum, attr.RemainingValue, bd.V)
	}
	return nil
}
