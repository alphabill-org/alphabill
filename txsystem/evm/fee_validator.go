package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
)

func isFeeCreditTx(tx *types.TransactionOrder) bool {
	typeUrl := tx.PayloadType()
	return typeUrl == fc.PayloadTypeAddFeeCredit ||
		typeUrl == fc.PayloadTypeCloseFeeCredit
}

func checkFeeAccountBalance(state *state.State, execPredicate func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, exeCtx predicates.TxContext) error) genericTransactionValidator {
	return func(ctx *TxValidationContext) error {
		if isFeeCreditTx(ctx.Tx) {
			addr, err := getAddressFromPredicateArg(ctx.Tx.OwnerProof)
			if err != nil {
				return fmt.Errorf("failed to extract address from public key bytes, %w", err)
			}
			u, _ := state.GetUnit(addr.Bytes(), false)
			if u == nil && ctx.Tx.PayloadType() == fc.PayloadTypeCloseFeeCredit {
				return fmt.Errorf("no fee credit info found for unit %X", ctx.Tx.UnitID())
			}
			if u == nil && ctx.Tx.PayloadType() == fc.PayloadTypeAddFeeCredit {
				// account creation
				return nil
			}
			// owner proof verifies correctly
			if err = execPredicate(u.Bearer(), ctx.Tx.OwnerProof, ctx.Tx, ctx); err != nil {
				return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x]",
					err, ctx.Tx.OwnerProof, u.Bearer())
			}
		}
		return nil
	}
}
