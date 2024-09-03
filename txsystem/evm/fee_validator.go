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

func checkFeeAccountBalance(state *state.State, execPredicate func(predicate types.PredicateBytes, args []byte, tx *types.TransactionOrder, exeCtx predicates.TxContext) error) genericTransactionValidator {
	return func(ctx *TxValidationContext) error {
		if !isFeeCreditTx(ctx.Tx) {
			return nil
		}
		ownerProof, err := getOwnerProof(ctx.Tx)
		if err != nil {
			return fmt.Errorf("failed to parse owner proof: %w", err)
		}
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
		ownerPredicate := u.Owner()
		if err = execPredicate(ownerPredicate, ownerProof, ctx.Tx, ctx); err != nil {
			return fmt.Errorf("invalid owner proof: %w [authProof.OwnerProof=0x%x unit.Owner=0x%x]", err, ownerProof, ownerPredicate)
		}
		return nil
	}
}

func getOwnerProof(tx *types.TransactionOrder) ([]byte, error) {
	var ownerProof []byte
	if tx.PayloadType() == fc.PayloadTypeAddFeeCredit {
		var authProof *fc.AddFeeCreditAuthProof
		if err := types.Cbor.Unmarshal(tx.AuthProof, &authProof); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s auth proof: %w", tx.PayloadType(), err)
		}
		ownerProof = authProof.OwnerProof
	} else if tx.PayloadType() == fc.PayloadTypeCloseFeeCredit {
		var authProof *fc.CloseFeeCreditAuthProof
		if err := types.Cbor.Unmarshal(tx.AuthProof, &authProof); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s auth proof: %w", tx.PayloadType(), err)
		}
		ownerProof = authProof.OwnerProof
	} else {
		return nil, fmt.Errorf("unsupported transaction type: %s", tx.PayloadType())
	}
	return ownerProof, nil
}
