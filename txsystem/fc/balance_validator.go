package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
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
	if err := f.execPredicate(unit.Bearer(), getFeeProof(tx), tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating fee proof: %w", err)
	}
	return nil
}

// getFeeProof returns tx.FeeProof if it exists or tx.OwnerProof if it does not exist
func getFeeProof(tx *types.TransactionOrder) []byte {
	if len(tx.FeeProof) > 0 {
		return tx.FeeProof
	}
	return tx.OwnerProof
}
