package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
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

func checkFeeAccountBalance(state *state.State, execPredicate func(predicate types.PredicateBytes, args []byte, sigBytes []byte, exeCtx predicates.TxContext) error) genericTransactionValidator {
	return func(ctx *TxValidationContext) error {
		if isFeeCreditTx(ctx.Tx) {
			// TODO extract owner proof from fee tx
			//addr, err := getAddressFromPredicateArg(ctx.Tx.OwnerProof)
			ownerProof := templates.EmptyArgument()
			addr, err := getAddressFromPredicateArg(ownerProof)
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
			sigBytes, err := ctx.Tx.PayloadBytes()
			if err != nil {
				return fmt.Errorf("failed to get signature bytes from the transaction: %w", err)
			}
			if err = execPredicate(u.Owner(), ownerProof, sigBytes, ctx); err != nil {
				return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x]",
					err, ownerProof, u.Owner())
			}
		}
		return nil
	}
}
