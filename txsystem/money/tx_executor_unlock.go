package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/types"
)

var (
	ErrBillUnlocked = errors.New("bill is already unlocked")
)

func handleUnlockTx(s *state.State, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[UnlockAttributes] {
	return func(tx *types.TransactionOrder, attr *UnlockAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()
		unit, _ := s.GetUnit(unitID, false)
		if unit == nil {
			return nil, fmt.Errorf("unlock tx: unit not found %X", tx.UnitID())
		}
		billData, ok := unit.Data().(*BillData)
		if !ok {
			return nil, errors.New("unlock tx: invalid unit type")
		}
		if err := validateUnlockTx(attr, billData); err != nil {
			return nil, fmt.Errorf("unlock tx: validation failed: %w", err)
		}
		// unlock the unit
		action := state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
			newBillData, ok := data.(*BillData)
			if !ok {
				return nil, fmt.Errorf("unlock tx: unit %v does not contain bill data", unitID)
			}
			newBillData.Locked = 0
			newBillData.T = currentBlockNumber
			newBillData.Backlink = tx.Hash(hashAlgorithm)
			return newBillData, nil
		})
		if err := s.Apply(action); err != nil {
			return nil, fmt.Errorf("unlock tx: failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: feeCalc(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func validateUnlockTx(attr *UnlockAttributes, bd *BillData) error {
	if attr == nil {
		return ErrTxAttrNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if !bd.IsLocked() {
		return ErrBillUnlocked
	}
	if !bytes.Equal(attr.Backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	return nil
}
