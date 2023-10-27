package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/state"
)

var ErrInvalidLockStatus = errors.New("invalid lock status: expected non-zero value, got zero value")

func handleLockTx(s *state.State, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[LockAttributes] {
	return func(tx *types.TransactionOrder, attr *LockAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()
		unit, _ := s.GetUnit(unitID, false)
		if unit == nil {
			return nil, fmt.Errorf("lock tx: unit not found %X", tx.UnitID())
		}
		billData, ok := unit.Data().(*BillData)
		if !ok {
			return nil, errors.New("lock tx: invalid unit type")
		}
		if err := validateLockTx(attr, billData); err != nil {
			return nil, fmt.Errorf("lock tx: validation failed: %w", err)
		}
		// lock the unit
		action := state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
			newBillData, ok := data.(*BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			newBillData.Locked = attr.LockStatus
			newBillData.T = currentBlockNumber
			newBillData.Backlink = tx.Hash(hashAlgorithm)
			return newBillData, nil
		})
		if err := s.Apply(action); err != nil {
			return nil, fmt.Errorf("lock tx: failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: feeCalc(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func validateLockTx(attr *LockAttributes, bd *BillData) error {
	if attr == nil {
		return ErrTxAttrNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if bd.IsLocked() {
		return ErrBillLocked
	}
	if attr.LockStatus == 0 {
		return ErrInvalidLockStatus
	}
	if !bytes.Equal(attr.Backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	return nil
}
