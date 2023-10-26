package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/predicates"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	fcunit "github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
)

func checkFeeCreditBalance(s *state.State, feeCalculator FeeCalculator) txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		if !transactions.IsFeeCreditTx(ctx.Tx) {
			clientMetadata := ctx.Tx.Payload.ClientMetadata

			// 5. ExtrType(ιf) = fcr ∧ N[ιf] != ⊥ – the fee payer has credit in this system
			feeCreditRecordID := clientMetadata.FeeCreditRecordID
			if len(feeCreditRecordID) == 0 {
				return errors.New("fee credit record missing")
			}

			unit, _ := s.GetUnit(feeCreditRecordID, false)
			if unit == nil {
				return errors.New("fee credit record unit is nil")
			}
			fcr, ok := unit.Data().(*fcunit.FeeCreditRecord)
			if !ok {
				return errors.New("invalid fee credit record type")
			}

			// 6. if the transaction has a fee authorization proof,
			//    it must satisfy the owner_bytes condition of the fee credit record
			// 7. if the transaction does not have a separate fee authorization proof,
			//    the owner_bytes proof of the whole transaction must also satisfy the owner_bytes condition of the fee credit record
			feeProof := getFeeProof(ctx)

			sigBytes, err := ctx.Tx.PayloadBytes()
			if err != nil {
				return fmt.Errorf("failed to get payload bytes: %w", err)
			}
			if err := predicates.RunPredicate(unit.Bearer(), feeProof, sigBytes); err != nil {
				return fmt.Errorf("invalid fee proof: %w [txFeeProof=0x%x unitOwnerCondition=0x%x sigData=0x%x]",
					err, feeProof, unit.Bearer(), sigBytes)
			}

			// 8. the maximum permitted transaction cost does not exceed the fee credit balance
			if fcr.Balance < ctx.Tx.Payload.ClientMetadata.MaxTransactionFee {
				return fmt.Errorf("the max tx fee cannot exceed fee credit balance. FC balance %d vs max tx fee %d", fcr.Balance, ctx.Tx.Payload.ClientMetadata.MaxTransactionFee)
			}
		}

		// 9. the actual transaction cost does not exceed the maximum permitted by the user
		if feeCalculator() > ctx.Tx.Payload.ClientMetadata.MaxTransactionFee {
			return errors.New("the tx fee cannot exceed the max specified fee")
		}
		return nil
	}
}

// getFeeProof returns tx.FeeProof if it exists or tx.OwnerProof if it does not exist
func getFeeProof(ctx *txsystem.TxValidationContext) []byte {
	feeProof := ctx.Tx.FeeProof
	if len(feeProof) > 0 {
		return feeProof
	}
	return ctx.Tx.OwnerProof
}
