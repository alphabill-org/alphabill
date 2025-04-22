package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *Module) executeSplitTx(tx *types.TransactionOrder, attr *money.SplitAttributes, authProof *money.SplitAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	targetUnitIDs := []types.UnitID{unitID}
	// add new units
	var actions []state.Action
	var sum uint64
	var ok bool
	idGen := money.PrndSh(tx)
	for _, targetUnit := range attr.TargetUnits {
		newUnitID, err := m.pdr.ComposeUnitID(types.ShardID{}, money.BillUnitType, idGen)
		if err != nil {
			return nil, fmt.Errorf("failed to generate new bill id: %w", err)
		}
		targetUnitIDs = append(targetUnitIDs, newUnitID)
		newUnitData := money.NewBillData(targetUnit.Amount, targetUnit.OwnerPredicate)
		actions = append(actions, state.AddOrPromoteUnit(newUnitID, newUnitData))
		if sum, ok = util.SafeAdd(sum, targetUnit.Amount); !ok {
			return nil, errors.New("overflow when summing target unit amounts")
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
	if err = m.execPredicate(unitData.Owner(), authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	return nil
}

func (m *Module) splitTxTargetUnits(tx *types.TransactionOrder, attr *money.SplitAttributes, _ *money.SplitAuthProof, _ txtypes.ExecutionContext) ([]types.UnitID, error) {
	targetUnits := make([]types.UnitID, 0, len(attr.TargetUnits)+1)
	targetUnits = append(targetUnits, tx.UnitID)
	idGen := money.PrndSh(tx)
	for range attr.TargetUnits {
		newUnitID, err := m.pdr.ComposeUnitID(types.ShardID{}, money.BillUnitType, idGen)
		if err != nil {
			return nil, fmt.Errorf("failed to generate new bill id: %w", err)
		}
		targetUnits = append(targetUnits, newUnitID)
	}
	return targetUnits, nil
}

func validateSplit(data types.UnitData, attr *money.SplitAttributes) error {
	bd, ok := data.(*money.BillData)
	if !ok {
		return errors.New("invalid data type, unit is not of BillData type")
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
		if sum, ok = util.SafeAdd(sum, targetUnit.Amount); !ok {
			return errors.New("overflow when summing target unit amounts")
		}
	}
	if sum >= bd.Value {
		return fmt.Errorf("the sum of the values to be transferred must be less than the value of the bill; sum=%d billValue=%d", sum, bd.Value)
	}
	return nil
}
