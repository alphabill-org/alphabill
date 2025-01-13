package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

func (f *FeeCreditModule) executeCloseFC(tx *types.TransactionOrder, _ *fc.CloseFeeCreditAttributes, _ *fc.CloseFeeCreditAuthProof, execCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	updateDataFn := state.UpdateUnitData(tx.UnitID,
		func(data types.UnitData) (types.UnitData, error) {
			fcr, ok := data.(*fc.FeeCreditRecord)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain fee credit record", tx.UnitID)
			}
			fcr.Balance = 0
			fcr.Counter += 1
			return fcr, nil
		},
	)
	if err := f.state.Apply(updateDataFn); err != nil {
		return nil, fmt.Errorf("closeFC: state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: execCtx.CalculateCost(), TargetUnits: []types.UnitID{tx.UnitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeCreditModule) validateCloseFC(tx *types.TransactionOrder, attr *fc.CloseFeeCreditAttributes, authProof *fc.CloseFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	// there’s no fee credit reference or separate fee authorization proof
	if err := ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	// ι identifies an existing fee credit record
	// ExtrType(P.ι) = fcr – target unit is a fee credit record
	// S.N[P.ι] != ⊥ - ι identifies an existing fee credit record
	fcr, err := parseFeeCreditRecord(&f.pdr, tx.UnitID, f.feeCreditRecordUnitType, f.state)
	if err != nil {
		return fmt.Errorf("fee credit error: %w", err)
	}
	// verify the fee credit record is not locked
	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	// target unit list is empty
	if err = ValidateCloseFC(attr, fcr); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	// validate owner predicate
	// S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner predicate matches
	if err = f.execPredicate(fcr.OwnerPredicate, authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return fmt.Errorf("executing fee credit record owner predicate: %w", err)
	}
	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if err = VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, fcr.Balance); err != nil {
		return fmt.Errorf("not enough funds: %w", err)
	}
	return nil
}
