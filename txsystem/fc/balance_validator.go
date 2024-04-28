package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"
)

/*
CheckFeeCreditBalance implements the fee credit verification steps listed in the
Yellowpaper "Valid Transaction Orders" chapter.
*/
func (f *FeeCredit) CheckFeeCreditBalance(tx *types.TransactionOrder) error {
	if !fc.IsFeeCreditTx(tx) {
		clientMetadata := tx.Payload.ClientMetadata

		// 5. ExtrType(ιf) = fcr ∧ N[ιf] != ⊥ – the fee payer has credit in this system
		feeCreditRecordID := clientMetadata.FeeCreditRecordID
		if len(feeCreditRecordID) == 0 {
			return errors.New("fee credit record missing")
		}

		unit, _ := f.state.GetUnit(feeCreditRecordID, false)
		if unit == nil {
			return errors.New("fee credit record unit is nil")
		}
		fcr, ok := unit.Data().(*fc.FeeCreditRecord)
		if !ok {
			return errors.New("invalid fee credit record type")
		}

		// 6. if the transaction has a fee authorization proof,
		//    it must satisfy the owner_bytes condition of the fee credit record
		// 7. if the transaction does not have a separate fee authorization proof,
		//    the owner_bytes proof of the whole transaction must also satisfy the owner_bytes condition of the fee credit record
		if err := f.execPredicate(unit.Bearer(), getFeeProof(tx), tx); err != nil {
			return fmt.Errorf("evaluating fee proof: %w", err)
		}

		// 8. the maximum permitted transaction cost does not exceed the fee credit balance
		if fcr.Balance < tx.Payload.ClientMetadata.MaxTransactionFee {
			return fmt.Errorf("the max tx fee cannot exceed fee credit balance. FC balance %d vs max tx fee %d", fcr.Balance, tx.Payload.ClientMetadata.MaxTransactionFee)
		}
	}

	// 9. the actual transaction cost does not exceed the maximum permitted by the user
	if f.feeCalculator() > tx.Payload.ClientMetadata.MaxTransactionFee {
		return errors.New("the tx fee cannot exceed the max specified fee")
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
