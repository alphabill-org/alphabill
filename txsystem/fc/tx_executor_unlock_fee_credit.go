package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

func (f *FeeCredit) executeUnlockFC(tx *types.TransactionOrder, _ *fc.UnlockFeeCreditAttributes, execCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	fee := execCtx.CalculateCost()
	updateFunc := state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			fcr, ok := data.(*fc.FeeCreditRecord)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain fee credit record", unitID)
			}
			fcr.Balance -= fee
			fcr.Counter += 1
			fcr.Locked = 0
			return fcr, nil
		})
	if err := f.state.Apply(updateFunc); err != nil {
		return nil, fmt.Errorf("lockFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeCredit) validateUnlockFC(tx *types.TransactionOrder, attr *fc.UnlockFeeCreditAttributes, _ txtypes.ExecutionContext) error {
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
	if attr.Counter != fcr.GetCounter() {
		return fmt.Errorf("the transaction counter does not equal with the fee credit record counter: "+
			"got %d expected %d", attr.Counter, fcr.GetCounter())
	}
	// the transaction fee can’t exceed the record balance
	if err = VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, fcr.Balance); err != nil {
		return fmt.Errorf("not enough funds: %w", err)
	}
	// the unit is in locked state --- should this be an error should we handle this as nop?
	if !fcr.IsLocked() {
		return errors.New("fee credit record is already unlocked")
	}

	return nil
}
