package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeCreditModule) executeLockFC(tx *types.TransactionOrder, attr *fc.LockFeeCreditAttributes, _ *fc.LockFeeCreditAuthProof, execCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	fee := execCtx.CalculateCost()
	if err := f.state.Apply(state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			fcr, ok := data.(*fc.FeeCreditRecord)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain fee credit record", unitID)
			}
			fcr.Balance -= fee
			fcr.Counter += 1
			fcr.Locked = attr.LockStatus
			return fcr, nil
		})); err != nil {
		return nil, fmt.Errorf("lockFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeCreditModule) validateLockFC(tx *types.TransactionOrder, attr *fc.LockFeeCreditAttributes, authProof *fc.LockFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	// there’s no fee credit reference or separate fee authorization proof
	if err := ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	// ι identifies an existing fee credit record
	fcr, err := parseFeeCreditRecord(&f.pdr, tx.UnitID, f.feeCreditRecordUnitType, f.state)
	if err != nil {
		return fmt.Errorf("get unit error: %w", err)
	}
	// the transaction follows the previous valid transaction with the record
	if attr.Counter != fcr.GetCounter() {
		return fmt.Errorf("the transaction counter does not equal with the fee credit record counter: "+
			"got %d expected %d", attr.Counter, fcr.GetCounter())
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
	// validate owner
	if err = f.execPredicate(fcr.OwnerPredicate, authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return fmt.Errorf("executing fee credit predicate: %w", err)
	}
	return nil
}
