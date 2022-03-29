package txsystem

import (
	"bytes"
	"crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
	"github.com/holiman/uint256"
)

var (
	ErrInvalidDataType = errors.New("invalid data type")

	ErrInvalidBacklink    = errors.New("transaction backlink must be equal to bill backlink")
	ErrInvalidBillValue   = errors.New("transaction value must be equal to bill value")
	ErrTransactionExpired = errors.New("transaction timeout must be greater than current block height")

	// swap tx specific validity conditions
	ErrSwapInvalidTargetValue        = errors.New("target value of the bill must be equal to the sum of the target values of succeeded payments in swap transaction")
	ErrSwapInsufficientDCMoneySupply = errors.New("insufficient DC-money supply")
	ErrSwapBillAlreadyExists         = errors.New("swapped bill id already exists")
	ErrSwapInvalidBillIdentifiers    = errors.New("all bill identifiers in dust transfer orders must exist in transaction bill identifiers")
	ErrSwapInvalidBillId             = errors.New("bill id is not properly computed")
	ErrSwapInvalidNonce              = errors.New("dust transfer orders do not contain proper nonce")
	ErrSwapInvalidTargetBearer       = errors.New("dust transfer orders do not contain proper target bearer")
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
		return ErrInvalidDataType
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
		return ErrInvalidDataType
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

func validateSwap(tx Swap, hashAlgorithm crypto.Hash) error {
	// 1. target value of the bill is the sum of the target values of succeeded payments in P
	expectedSum := tx.TargetValue()
	actualSum := sumDcTransferValues(tx)
	if expectedSum != actualSum {
		return ErrSwapInvalidTargetValue
	}

	// 2. there is suffiecient DC-money supply
	// 3. there exists no bill with identifier
	// checked in moneySchemeState#validateSwap method

	// 4. all bill ids in dust transfer orders are elements of bill ids in swap tx
	for _, dcTx := range tx.DCTransfers() {
		exists := billIdInList(dcTx.UnitId(), tx.BillIdentifiers())
		if !exists {
			return ErrSwapInvalidBillIdentifiers
		}
	}

	// 5. new bill id is properly computed ι=h(ι1,...,ιm)
	expectedBillId := hashBillIds(tx, hashAlgorithm)
	if !bytes.Equal(tx.UnitId().Bytes(), expectedBillId) {
		return ErrSwapInvalidBillId
	}

	// 6. bills were transfered to DC (validate dc transfer type)
	// already checked on language/protobuf level

	// 7. bill transfer orders contain proper nonce
	// 8. bill transfer orders contain proper target bearer
	for _, dcTx := range tx.DCTransfers() {
		if !bytes.Equal(dcTx.Nonce(), tx.UnitId().Bytes()) {
			return ErrSwapInvalidNonce
		}
		if !bytes.Equal(dcTx.TargetBearer(), tx.OwnerCondition()) {
			return ErrSwapInvalidTargetBearer
		}
	}

	// 9. verify owner
	// done in validateGenericTransaction function

	// TODO 10. verify ledger proof https://guardtime.atlassian.net/browse/AB-50

	return nil
}

func hashBillIds(tx Swap, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	for _, billId := range tx.BillIdentifiers() {
		hasher.Write(billId.Bytes())
	}
	return hasher.Sum(nil)
}

func billIdInList(billId *uint256.Int, billIds []*uint256.Int) bool {
	for _, bId := range billIds {
		if bId.Eq(billId) {
			return true
		}
	}
	return false
}

func sumDcTransferValues(tx Swap) uint64 {
	sum := uint64(0)
	for _, dcTx := range tx.DCTransfers() {
		sum += dcTx.TargetValue()
	}
	return sum
}
