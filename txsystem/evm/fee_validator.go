package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/validator/internal/predicates"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	"github.com/alphabill-org/alphabill/validator/internal/types"
)

func isFeeCreditTx(tx *types.TransactionOrder) bool {
	typeUrl := tx.PayloadType()
	return typeUrl == transactions.PayloadTypeAddFeeCredit ||
		typeUrl == transactions.PayloadTypeCloseFeeCredit
}

func checkFeeAccountBalance(state *state.State) txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		if isFeeCreditTx(ctx.Tx) {
			addr, err := getAddressFromPredicateArg(ctx.Tx.OwnerProof)
			if err != nil {
				return fmt.Errorf("failed to extract address from public key bytes, %w", err)
			}
			u, _ := state.GetUnit(addr.Bytes(), false)
			if u == nil && ctx.Tx.PayloadType() == transactions.PayloadTypeCloseFeeCredit {
				return fmt.Errorf("no fee credit info found for unit %X", ctx.Tx.UnitID())
			}
			if u == nil && ctx.Tx.PayloadType() == transactions.PayloadTypeAddFeeCredit {
				// account creation
				return nil
			}
			// owner proof verifies correctly
			payloadBytes, err := ctx.Tx.PayloadBytes()
			if err != nil {
				return fmt.Errorf("failed to marshal payload bytes: %w", err)
			}

			if err = predicates.RunPredicate(u.Bearer(), ctx.Tx.OwnerProof, payloadBytes); err != nil {
				return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x sigData=0x%x]",
					err, ctx.Tx.OwnerProof, u.Bearer(), payloadBytes)
			}
		}
		return nil
	}
}
