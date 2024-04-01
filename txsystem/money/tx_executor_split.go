package money

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

func HashForIDCalculation(idBytes []byte, attr []byte, timeout uint64, idx uint32, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	hasher.Write(idBytes)
	hasher.Write(attr)
	hasher.Write(util.Uint64ToBytes(timeout))
	hasher.Write(util.Uint32ToBytes(idx))
	return hasher.Sum(nil)
}

func (m *Module) handleSplitTx() txsystem.GenericExecuteFunc[SplitAttributes] {
	return func(tx *types.TransactionOrder, attr *SplitAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		if err := m.validateSplitTx(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid split transaction: %w", err)
		}
		unitID := tx.UnitID()
		targetUnitIDs := []types.UnitID{unitID}

		// add new units
		var actions []state.Action
		for i, targetUnit := range attr.TargetUnits {
			newUnitID := NewBillID(unitID, HashForIDCalculation(unitID, tx.Payload.Attributes, tx.Timeout(), uint32(i), m.hashAlgorithm))
			targetUnitIDs = append(targetUnitIDs, newUnitID)
			actions = append(actions, state.AddUnit(
				newUnitID,
				targetUnit.OwnerCondition,
				&BillData{
					V:       targetUnit.Amount,
					T:       currentBlockNumber,
					Counter: 0,
				}))
		}

		// update existing unit
		actions = append(actions, state.UpdateUnitData(unitID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
				}
				return &BillData{
					V:       attr.RemainingValue,
					T:       currentBlockNumber,
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
}

func (m *Module) validateSplitTx(tx *types.TransactionOrder, attr *SplitAttributes) error {
	unit, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	if err := m.execPredicate(unit.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	return validateSplit(unit.Data(), attr)
}

func validateSplit(data state.UnitData, attr *SplitAttributes) error {
	bd, ok := data.(*BillData)
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
