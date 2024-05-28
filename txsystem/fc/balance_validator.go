package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

/*
CheckFeeCreditBalance implements the fee credit verification steps listed in the
Yellowpaper "Valid Transaction Orders" chapter. and fee credit is missing
*/
func (f *FeeCredit) CheckFeeCreditBalance(exeCtx txsystem.ExecutionContext, tx *types.TransactionOrder) error {
	clientMetadata := tx.Payload.ClientMetadata

	// 1. ExtrType(ιf) = fcr ∧ N[ιf] != ⊥ – the fee payer has credit in this system
	feeCreditRecordID := clientMetadata.FeeCreditRecordID
	if len(feeCreditRecordID) == 0 {
		return errors.New("fee credit record missing")
	}
	if !types.UnitID(feeCreditRecordID).HasType(f.feeCreditRecordUnitType) {
		return errors.New("invalid fee credit record id type")
	}
	unit, _ := f.state.GetUnit(feeCreditRecordID, false)
	if unit == nil {
		return errors.New("fee credit record unit is nil")
	}
	fcr, ok := unit.Data().(*fc.FeeCreditRecord)
	if !ok {
		return errors.New("invalid fee credit record type")
	}
	// 2. the maximum permitted transaction cost does not exceed the fee credit balance
	if fcr.Balance < tx.Payload.ClientMetadata.MaxTransactionFee {
		return fmt.Errorf("the max tx fee cannot exceed fee credit balance. FC balance %d vs max tx fee %d", fcr.Balance, tx.Payload.ClientMetadata.MaxTransactionFee)
	}
	// 3. if the transaction has a fee authorization proof,
	// it must satisfy the owner predicate of the fee credit record
	if err := f.execPredicate(unit.Bearer(), tx.FeeProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating fee proof: %w", err)
	}
	return nil
}

func (f *FeeCredit) CheckFeeCreditTx(tx *types.TransactionOrder, exeCtx txsystem.ExecutionContext) error {
	if err := ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("fee credit tx validation error: %w", err)
	}
	unit, err := f.state.GetUnit(tx.UnitID(), false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("state read error: %w", err)
	}
	// special case for 1st addFC
	if errors.Is(err, avl.ErrNotFound) {
		// if not adding first fee credit
		if tx.PayloadType() != fc.PayloadTypeAddFeeCredit {
			return fmt.Errorf("no fee credit unit found")
		}
		attr := &fc.AddFeeCreditAttributes{}
		if err = tx.UnmarshalAttributes(attr); err != nil {
			return fmt.Errorf("failed to unmarshal add fee credit payload: %w", err)
		}
		// S.N[P.ι] == ⊥ ∧ VerifyOwner(P.A.φ, P, P.s) = 1 – if the target does not exist, the owner proof must verify
		if err = f.execPredicate(attr.FeeCreditOwnerCondition, tx.OwnerProof, tx, exeCtx); err != nil {
			return fmt.Errorf("executing fee credit predicate: %w", err)
		}
	} else {
		// check owner condition
		if err = f.execPredicate(unit.Bearer(), tx.FeeProof, tx, exeCtx); err != nil {
			return fmt.Errorf("evaluating fee proof: %w", err)
		}
	}
	return nil
}
