package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
)

func (f *FeeCredit) executeCloseFC(tx *types.TransactionOrder, attr *fc.CloseFeeCreditAttributes, _ txsystem.ExecutionContext) (*types.ServerMetadata, error) {
	decrCreditFn := unit.DecrCredit(tx.UnitID(), attr.Amount)
	updateDataFn := state.UpdateUnitData(tx.UnitID(),
		func(data types.UnitData) (types.UnitData, error) {
			fcr, ok := data.(*fc.FeeCreditRecord)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain fee credit record", tx.UnitID())
			}
			fcr.Backlink = tx.Hash(f.hashAlgorithm)
			return fcr, nil
		})
	if err := f.state.Apply(decrCreditFn, updateDataFn); err != nil {
		return nil, fmt.Errorf("closeFC: state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: f.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeCredit) validateCloseFC(tx *types.TransactionOrder, attr *fc.CloseFeeCreditAttributes, _ txsystem.ExecutionContext) error {
	// there’s no fee credit reference or separate fee authorization proof
	if err := ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	// ι identifies an existing fee credit record
	// ExtrType(P.ι) = fcr – target unit is a fee credit record
	// S.N[P.ι] != ⊥ - ι identifies an existing fee credit record
	fcr, _, err := parseFeeCreditRecord(tx.UnitID(), f.feeCreditRecordUnitType, f.state)
	if err != nil {
		return fmt.Errorf("fee credit error: %w", err)
	}
	// verify the fee credit record is not locked
	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	// target unit list is empty
	if err = ValidateCloseFC(attr, fcr); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if err = VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, fcr.Balance); err != nil {
		return fmt.Errorf("not enough funds: %w", err)
	}
	return nil
}
