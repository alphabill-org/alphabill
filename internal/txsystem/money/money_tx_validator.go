package money

import (
	"bytes"
	"crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rma"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

var (
	ErrInvalidBillValue = errors.New("transaction value must be equal to bill value")

	// swap tx specific validity conditions
	ErrSwapInvalidTargetValue        = errors.New("target value of the bill must be equal to the sum of the target values of succeeded payments in swap transaction")
	ErrSwapInsufficientDCMoneySupply = errors.New("insufficient DC-money supply")
	ErrSwapBillAlreadyExists         = errors.New("swapped bill id already exists")
	ErrSwapInvalidBillIdentifiers    = errors.New("all bill identifiers in dust transfer orders must exist in transaction bill identifiers")
	ErrSwapInvalidBillId             = errors.New("bill id is not properly computed")
	ErrSwapDustTransfersInvalidOrder = errors.New("transfer orders are not listed in strictly increasing order of bill identifiers")
	ErrSwapInvalidNonce              = errors.New("dust transfer orders do not contain proper nonce")
	ErrSwapInvalidTargetBearer       = errors.New("dust transfer orders do not contain proper target bearer")
)

func validateTransfer(data rma.UnitData, tx Transfer) error {
	return validateAnyTransfer(data, tx.Backlink(), tx.TargetValue())
}

func validateTransferDC(data rma.UnitData, tx TransferDC) error {
	return validateAnyTransfer(data, tx.Backlink(), tx.TargetValue())
}

func validateAnyTransfer(data rma.UnitData, backlink []byte, targetValue uint64) error {
	bd, ok := data.(*BillData)
	if !ok {
		return txsystem.ErrInvalidDataType
	}
	if !bytes.Equal(backlink, bd.Backlink) {
		return txsystem.ErrInvalidBacklink
	}
	if targetValue != bd.V {
		return ErrInvalidBillValue
	}
	return nil
}

func validateSplit(data rma.UnitData, tx Split) error {
	bd, ok := data.(*BillData)
	if !ok {
		return txsystem.ErrInvalidDataType
	}
	if !bytes.Equal(tx.Backlink(), bd.Backlink) {
		return txsystem.ErrInvalidBacklink
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
	// checked in moneyTxSystem#validateSwap method

	// 4. all bill ids in dust transfer orders are elements of bill ids in swap tx
	for _, dcTx := range tx.DCTransfers() {
		exists := billIdInList(dcTx.UnitID(), tx.BillIdentifiers())
		if !exists {
			return ErrSwapInvalidBillIdentifiers
		}
	}

	// 5. new bill id is properly computed ι=h(ι1,...,ιm)
	expectedBillId := hashBillIds(tx, hashAlgorithm)
	unitIdBytes := tx.UnitID().Bytes32()
	if !bytes.Equal(unitIdBytes[:], expectedBillId) {
		return ErrSwapInvalidBillId
	}

	// 6. bills were transfered to DC (validate dc transfer type)
	// already checked on language/protobuf level

	// 7. bill transfer orders are listed in strictly increasing order of bill identifiers
	// (in particular, this ensures that no bill can be included multiple times)
	// 8. bill transfer orders contain proper nonce
	// 9. bill transfer orders contain proper target bearer
	var prevDcTx TransferDC
	for i, dcTx := range tx.DCTransfers() {
		if i > 0 {
			if !dcTx.UnitID().Gt(prevDcTx.UnitID()) {
				return ErrSwapDustTransfersInvalidOrder
			}
		}
		if !bytes.Equal(dcTx.Nonce(), unitIdBytes[:]) {
			return ErrSwapInvalidNonce
		}
		if !bytes.Equal(dcTx.TargetBearer(), tx.OwnerCondition()) {
			return ErrSwapInvalidTargetBearer
		}
		prevDcTx = dcTx
	}

	// 10. verify owner
	// done in validateGenericTransaction function

	// TODO 11. verify ledger proof https://guardtime.atlassian.net/browse/AB-50

	return nil
}

func hashBillIds(tx Swap, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	for _, billId := range tx.BillIdentifiers() {
		bytes32 := billId.Bytes32()
		hasher.Write(bytes32[:])
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
