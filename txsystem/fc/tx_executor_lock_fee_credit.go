package fc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (f *FeeCredit) executeLockFC(tx *types.TransactionOrder, attr *fc.LockFeeCreditAttributes, _ txsystem.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	fee := f.feeCalculator()
	txHash := tx.Hash(f.hashAlgorithm)
	if err := f.state.Apply(state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			fcr, ok := data.(*fc.FeeCreditRecord)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain fee credit record", unitID)
			}
			fcr.Balance -= fee
			fcr.Backlink = txHash
			fcr.Locked = attr.LockStatus
			return fcr, nil
		})); err != nil {
		return nil, fmt.Errorf("lockFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeCredit) validateLockFC(tx *types.TransactionOrder, attr *fc.LockFeeCreditAttributes, exeCtx txsystem.ExecutionContext) error {
	// there’s no fee credit reference or separate fee authorization proof
	if err := ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	// ι identifies an existing fee credit record
	fcr, _, err := parseFeeCreditRecord(tx.UnitID(), f.feeCreditRecordUnitType, f.state)
	if err != nil {
		return fmt.Errorf("get unit error: %w", err)
	}
	// the transaction follows the previous valid transaction with the record,
	if !bytes.Equal(attr.Backlink, fcr.GetBacklink()) {
		return fmt.Errorf("the transaction backlink does not match with fee credit record backlink: "+
			"got %x expected %x", attr.Backlink, fcr.GetBacklink())
	}
	if err = VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, fcr.Balance); err != nil {
		return fmt.Errorf("not enough funds: %w", err)
	}
	// verify fcr is not already locked
	if fcr.IsLocked() {
		return errors.New("fee credit record is already locked")
	}
	// verify lock status is non-zero i.e. "locked"
	if attr.LockStatus == 0 {
		return errors.New("lock status must be non-zero value")
	}
	return nil
}
