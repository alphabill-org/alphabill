package txsystem

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
)

var (
	ErrTransactionExpired      = errors.New("transaction timeout must be greater than current block number")
	ErrInvalidSystemIdentifier = errors.New("error invalid system identifier")
	ErrInvalidDataType         = errors.New("invalid data type")
	ErrInvalidBacklink         = errors.New("transaction backlink must be equal to bill backlink")
)

type TxValidationContext struct {
	Tx               GenericTransaction
	Bd               *rma.Unit
	FeeCreditRecord  *rma.Unit
	SystemIdentifier []byte
	BlockNumber      uint64
	TxFee            uint64
}

func ValidateGenericTransaction(ctx *TxValidationContext) error {
	// 1. transaction is sent to this system
	if !bytes.Equal(ctx.Tx.SystemID(), ctx.SystemIdentifier) {
		return ErrInvalidSystemIdentifier
	}

	// 2. shard identifier is in this shard
	// TODO sharding

	// 3. transaction is not expired
	if ctx.BlockNumber >= ctx.Tx.Timeout() {
		return ErrTransactionExpired
	}

	// 4. owner proof verifies correctly
	if ctx.Bd != nil {
		err := script.RunScript(ctx.Tx.OwnerProof(), ctx.Bd.Bearer, ctx.Tx.SigBytes())
		if err != nil {
			return fmt.Errorf("invalid owner proof: %w", err)
		}
	}

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
		//    it must satisfy the owner condition of the fee credit record
		// 7. if the transaction does not have a separate fee authorization proof,
		//    the owner proof of the whole transaction must also satisfy the owner condition of the fee credit record
		feeProof := getFeeProof(ctx)
		err := script.RunScript(feeProof, ctx.FeeCreditRecord.Bearer, ctx.Tx.SigBytes())
		if err != nil {
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

	// 10. ψτ((P,s),S) – type-specific validity condition holds
	// verified in specfic transaction processing functions
	return nil
}

func isFeeCreditTx(tx GenericTransaction) bool {
	typeUrl := tx.ToProtoBuf().TransactionAttributes.TypeUrl
	return typeUrl == TypeURLTransferFeeCreditOrder ||
		typeUrl == TypeURLAddFeeCreditOrder ||
		typeUrl == TypeURLCloseFeeCreditOrder ||
		typeUrl == TypeURLReclaimFeeCreditOrder
}

// getFeeProof returns tx.FeeProof it it exists or tx.OwnerProof if it does not exist
func getFeeProof(ctx *TxValidationContext) []byte {
	feeProof := ctx.Tx.ToProtoBuf().FeeProof
	if len(feeProof) > 0 {
		return feeProof
	}
	return ctx.Tx.OwnerProof()
}
