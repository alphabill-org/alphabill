package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

func (m *Module) executeSplitTx(tx *types.TransactionOrder, attr *money.SplitAttributes, _ *money.SplitAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	targetUnitIDs := []types.UnitID{unitID}
	// add new units
	var actions []state.Action
	var sum uint64
	for i, targetUnit := range attr.TargetUnits {
		newUnitPart, err := money.HashForNewBillID(tx, uint32(i), m.hashAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("failed to generate hash for new bill id: %w", err)
		}
		newUnitID := money.NewBillID(unitID, newUnitPart)
		targetUnitIDs = append(targetUnitIDs, newUnitID)
		newUnitData := money.NewBillData(targetUnit.Amount, targetUnit.OwnerPredicate)
		actions = append(actions, state.AddUnit(newUnitID, newUnitData))
		sum, _, err = util.AddUint64(sum, targetUnit.Amount)
		if err != nil {
			return nil, fmt.Errorf("failed to add target unit amounts: %w", err)
		}
	}
	// update existing unit
	actions = append(actions, state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			bd.Value -= sum
			bd.Counter += 1
			return bd, nil
		},
	))
	// update state
	if err := m.state.Apply(actions...); err != nil {
		return nil, fmt.Errorf("state update failed: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: targetUnitIDs, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) validateSplitTx(tx *types.TransactionOrder, attr *money.SplitAttributes, authProof *money.SplitAuthProof, exeCtx txtypes.ExecutionContext) error {
	unit, err := m.state.GetUnit(tx.UnitID, false)
	if err != nil {
		return err
	}
	unitData := unit.Data()
	if err = validateSplit(unitData, attr); err != nil {
		return fmt.Errorf("split error: %w", err)
	}
	if err = m.execPredicate(unitData.Owner(), authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
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
		if len(targetUnit.OwnerPredicate) == 0 {
			return fmt.Errorf("target unit owner predicate is empty at index %d", i)
		}
		var err error
		sum, _, err = util.AddUint64(sum, targetUnit.Amount)
		if err != nil {
			return fmt.Errorf("failed to add target unit amounts: %w", err)
		}
	}
	if sum >= bd.Value {
		return fmt.Errorf("the sum of the values to be transferred must be less than the value of the bill; sum=%d billValue=%d", sum, bd.Value)
	}
	return nil
}
