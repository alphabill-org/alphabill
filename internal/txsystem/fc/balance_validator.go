package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
)

func BalanceValidator(ctx *txsystem.TxValidationContext) error {
	// fee authorization proof (conditions 5 to 8) do not apply for fee transactions
	if !isFeeCreditTx(ctx.Tx) {
		// 5. ExtrType(ιf) = fcr ∧ N[ιf] != ⊥ – the fee payer has credit in this system
		if ctx.FeeCreditRecord == nil {
			return errors.New("fee credit record is nil")
		}
		fcr, ok := ctx.FeeCreditRecord.Data.(*FeeCreditRecord)
		if !ok {
			return errors.New("invalid fee credit record type")
		}

		// 6. if the transaction has a fee authorization proof,
		//    it must satisfy the owner_bytes condition of the fee credit record
		// 7. if the transaction does not have a separate fee authorization proof,
		//    the owner_bytes proof of the whole transaction must also satisfy the owner_bytes condition of the fee credit record
		feeProof := getFeeProof(ctx)
		if err := script.RunScript(feeProof, ctx.FeeCreditRecord.Bearer, ctx.Tx.SigBytes()); err != nil {
			return fmt.Errorf("invalid fee proof: %w", err)
		}

		// 8. the maximum permitted transaction cost does not exceed the fee credit balance
		if fcr.Balance < ctx.Tx.ToProtoBuf().ClientMetadata.MaxFee {
			return errors.New("the max tx fee cannot exceed fee credit balance")
		}
	}

	// 9. the actual transaction cost does not exceed the maximum permitted by the user
	if ctx.TxFee > ctx.Tx.ToProtoBuf().ClientMetadata.MaxFee {
		return errors.New("the tx fee cannot exceed the max specified fee")
	}
	return nil
}

func isFeeCreditTx(tx txsystem.GenericTransaction) bool {
	typeUrl := tx.ToProtoBuf().TransactionAttributes.TypeUrl
	return typeUrl == transactions.TypeURLTransferFeeCreditOrder ||
		typeUrl == transactions.TypeURLAddFeeCreditOrder ||
		typeUrl == transactions.TypeURLCloseFeeCreditOrder ||
		typeUrl == transactions.TypeURLReclaimFeeCreditOrder
}

func getFeeProof(ctx *txsystem.TxValidationContext) []byte {
	feeProof := ctx.Tx.ToProtoBuf().FeeProof
	if len(feeProof) > 0 {
		return feeProof
	}
	return ctx.Tx.OwnerProof()
}
