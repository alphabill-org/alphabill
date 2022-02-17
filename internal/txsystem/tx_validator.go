package txsystem

import (
	"bytes"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
)

var (
	ErrInvalidBacklink    = errors.New("transaction backlink must be equal to bill backlink")
	ErrInvalidBillValue   = errors.New("transaction value must be equal to bill value")
	ErrTransactionExpired = errors.New("transaction timeout must be greater than current block height")
)

func validateGenericTransaction(gtx GenericTransaction, blockNumber uint64) error {
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

func validateTransfer(data state.UnitData, tx Transfer) error {
	return validateAnyTransfer(data, tx.Backlink(), tx.TargetValue())
}

func validateTransferDC(data state.UnitData, tx TransferDC) error {
	return validateAnyTransfer(data, tx.Backlink(), tx.TargetValue())
}

func validateAnyTransfer(data state.UnitData, backlink []byte, targetValue uint64) error {
	bd, ok := data.(*BillData)
	if !ok {
		return errors.New("invalid data type")
	}
	if !bytes.Equal(backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	if targetValue != bd.V {
		return ErrInvalidBillValue
	}
	return nil
}

func validateSplit(data state.UnitData, tx Split) error {
	bd, ok := data.(*BillData)
	if !ok {
		return errors.New("invalid data type")
	}
	if !bytes.Equal(tx.Backlink(), bd.Backlink) {
		return ErrInvalidBacklink
	}
	// amount does not exceed value of the bill
	if tx.Amount() >= bd.V {
		return ErrInvalidBillValue
	}
	// remaining value equals the previous value minus the amount
	if tx.RemainingValue() != bd.V-tx.Amount() {
		return ErrInvalidBillValue
	}
	return nil
}
