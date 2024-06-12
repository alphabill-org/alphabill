package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

/*
IsCredible implements the fee credit verification for ordinary transactions (everything else except fee credit txs)
*/
func (f *FeeCredit) IsCredible(exeCtx txtypes.ExecutionContext, tx *types.TransactionOrder) error {
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

func (f *FeeCredit) IsCredibleFC(exeCtx txtypes.ExecutionContext, tx *types.TransactionOrder) error {
	unit, err := f.state.GetUnit(tx.UnitID(), false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("state read error: %w", err)
	}
	if unit != nil {
		if err = f.execPredicate(unit.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
			return fmt.Errorf("evaluating fee proof: %w", err)
		}
		// if the unit is FCR, make sure the FCR account balance has enough funds
		if fcr, ok := unit.Data().(*fc.FeeCreditRecord); ok {
			// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
			if err = VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, fcr.Balance); err != nil {
				return fmt.Errorf("not enough funds: %w", err)
			}
		}
		return nil
	}
	// special case for 1st addFC
	// if not adding first fee credit
	if tx.PayloadType() != fc.PayloadTypeAddFeeCredit {
		return fmt.Errorf("fee credit unit not found")
	}
	attr := &fc.AddFeeCreditAttributes{}
	if err = tx.UnmarshalAttributes(attr); err != nil {
		return fmt.Errorf("failed to unmarshal add fee credit payload: %w", err)
	}
	if err = f.validateCreateFC(tx, attr, exeCtx); err != nil {
		return fmt.Errorf("add fee credit tx validation error: %w", err)
	}
	return nil
}
