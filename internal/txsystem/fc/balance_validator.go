package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/holiman/uint256"
)

func checkFeeCreditBalance(state *rma.Tree, feeCalculator FeeCalculator) txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		if !isFeeCreditTx(ctx.Tx) {
			clientMetadata := ctx.Tx.ToProtoBuf().ClientMetadata
			feeCreditRecordId := clientMetadata.FeeCreditRecordId
			if len(feeCreditRecordId) == 0 {
				return errors.New("fee credit record missing")
			}
			unit := getFeeCreditRecordUnit(clientMetadata, state)

			// 5. ExtrType(ιf) = fcr ∧ N[ιf] != ⊥ – the fee payer has credit in this system
			if unit == nil {
				return errors.New("fee credit record unit is nil")
			}
			fcr, ok := unit.Data.(*FeeCreditRecord)
			if !ok {
				return errors.New("invalid fee credit record type")
			}

			// 6. if the transaction has a fee authorization proof,
			//    it must satisfy the owner_bytes condition of the fee credit record
			// 7. if the transaction does not have a separate fee authorization proof,
			//    the owner_bytes proof of the whole transaction must also satisfy the owner_bytes condition of the fee credit record
			feeProof := getFeeProof(ctx)
			if err := script.RunScript(feeProof, unit.Bearer, ctx.Tx.SigBytes()); err != nil {
				return fmt.Errorf("invalid fee proof: %w", err)
			}

			// 8. the maximum permitted transaction cost does not exceed the fee credit balance
			if fcr.Balance < ctx.Tx.ToProtoBuf().ClientMetadata.MaxFee {
				return errors.New("the max tx fee cannot exceed fee credit balance")
			}
		}

		// 9. the actual transaction cost does not exceed the maximum permitted by the user
		if feeCalculator() > ctx.Tx.ToProtoBuf().ClientMetadata.MaxFee {
			return errors.New("the tx fee cannot exceed the max specified fee")
		}
		return nil
	}
}

func isFeeCreditTx(tx txsystem.GenericTransaction) bool {
	typeUrl := tx.ToProtoBuf().TransactionAttributes.TypeUrl
	return typeUrl == transactions.TypeURLTransferFeeCreditOrder ||
		typeUrl == transactions.TypeURLAddFeeCreditOrder ||
		typeUrl == transactions.TypeURLCloseFeeCreditOrder ||
		typeUrl == transactions.TypeURLReclaimFeeCreditOrder
}

// getFeeProof returns tx.FeeProof if it exists or tx.OwnerProof if it does not exist
func getFeeProof(ctx *txsystem.TxValidationContext) []byte {
	feeProof := ctx.Tx.ToProtoBuf().FeeProof
	if len(feeProof) > 0 {
		return feeProof
	}
	return ctx.Tx.OwnerProof()
}

func getFeeCreditRecordUnit(clientMD *txsystem.ClientMetadata, state *rma.Tree) *rma.Unit {
	var fcr *rma.Unit
	if len(clientMD.FeeCreditRecordId) > 0 {
		fcr, _ = state.GetUnit(uint256.NewInt(0).SetBytes(clientMD.FeeCreditRecordId))
	}
	return fcr
}
