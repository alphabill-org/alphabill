package txsystem

import (
	"bytes"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
)

var (
	ErrTransactionExpired      = errors.New("transaction timeout must be greater than current block height")
	ErrInvalidSystemIdentifier = errors.New("error invalid system identifier")
	ErrInvalidDataType         = errors.New("invalid data type")
	ErrInvalidBacklink         = errors.New("transaction backlink must be equal to bill backlink")
)

type TxValidationContext struct {
	Tx               transaction.GenericTransaction
	Bd               *state.Unit
	SystemIdentifier []byte
	BlockNumber      uint64
}

func ValidateGenericTransaction(ctx *TxValidationContext) error {
	// Let S=(α,SH,ιL,ιR,n,ιr,N,T,SD)be a state where N[ι]=(φ,D,x,V,h,ιL,ιR,d,b).
	// Signed transaction order(P,s), whereP=〈α,τ,ι,A,T0〉, isvalidif the next conditions hold:

	// 1. P.α=S.α – transaction is sent to this system
	if !bytes.Equal(ctx.Tx.SystemID(), ctx.SystemIdentifier) {
		return ErrInvalidSystemIdentifier
	}

	//2. (ιL=⊥ ∨ιL< ι)∧(ιR=⊥ ∨ι < ιR) – shard identifier is in this shard
	// TODO sharding

	//3. n < T0 – transaction is not expired
	if ctx.BlockNumber >= ctx.Tx.Timeout() {
		return ErrTransactionExpired
	}

	//4. N[ι]=⊥ ∨ VerifyOwner(N[ι].φ,P,s) = 1 – owner proof verifies correctly
	if ctx.Bd != nil {
		err := script.RunScript(ctx.Tx.OwnerProof(), ctx.Bd.Bearer, ctx.Tx.SigBytes())
		if err != nil {
			return err
		}
	}

	// 5.ψτ((P,s),S) – type-specific validity condition holds
	// verified in specfic transaction processing functions
	return nil
}
