package txsystem

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var (
	ErrTransactionExpired = errors.New("transaction timeout must be greater than current block height")
)

func ValidateGenericTransaction(gtx GenericTransaction, blockNumber uint64) error {
	// TODO general transaction validity functions
	//Let S=(α,SH,ιL,ιR,n,ιr,N,T,SD)be a state where N[ι]=(φ,D,x,V,h,ιL,ιR,d,b).
	//Signed transaction order(P,s), whereP=〈α,τ,ι,A,T0〉, isvalidif the next conditions hold:
	//1.P.α=S.α – transaction is sent to this system
	//2.(ιL=⊥ ∨ιL< ι)∧(ιR=⊥ ∨ι < ιR) – shard identifier is in this shard

	//3.n<T0 – transaction is not expired
	if blockNumber >= gtx.Timeout() {
		return ErrTransactionExpired
	}

	//4.N[ι]=⊥ ∨VerifyOwner(N[ι].φ,P,s)=1 – owner proof verifies correctly
	//5.ψτ((P,s),S) – type-specific validity condition holds
	return nil
}
