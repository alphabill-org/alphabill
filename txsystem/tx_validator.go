package txsystem

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
)

var (
	ErrTransactionExpired      = errors.New("transaction timeout must be greater than current block number")
	ErrInvalidSystemIdentifier = errors.New("error invalid system identifier")
)

type GenericTransactionValidator func(ctx *TxValidationContext) error

type TxValidationContext struct {
	Tx               *types.TransactionOrder
	Unit             *state.Unit
	SystemIdentifier types.SystemID
	BlockNumber      uint64
}

func ValidateGenericTransaction(ctx *TxValidationContext) error {
	// 1. transaction is sent to this system
	if ctx.Tx.SystemID() != ctx.SystemIdentifier {
		return ErrInvalidSystemIdentifier
	}

	// 2. shard identifier is in this shard
	// TODO sharding

	// 3. transaction is not expired
	if ctx.BlockNumber >= ctx.Tx.Timeout() {
		return ErrTransactionExpired
	}

	// 4. owner proof verifies correctly
	if ctx.Unit != nil {
		payloadBytes, err := ctx.Tx.PayloadBytes()
		if err != nil {
			return fmt.Errorf("failed to marshal payload bytes: %w", err)
		}

		if err = predicates.RunPredicate(ctx.Unit.Bearer(), ctx.Tx.OwnerProof, payloadBytes); err != nil {
			return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x sigData=0x%x]",
				err, ctx.Tx.OwnerProof, ctx.Unit.Bearer(), payloadBytes)
		}
	}
	return nil
}
