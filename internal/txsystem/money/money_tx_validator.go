package money

import (
	"bytes"
	"crypto"
	goerrors "errors"

	"github.com/holiman/uint256"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrInvalidBillValue = goerrors.New("transaction value must be equal to bill value")

	ErrSplitBillZeroAmount    = goerrors.New("when splitting an bill the value assigned to the new bill must be greater than zero")
	ErrSplitBillZeroRemainder = goerrors.New("when splitting an bill the remaining value of the bill must be greater than zero")

	// swap tx specific validity conditions
	ErrSwapInvalidTargetValue        = goerrors.New("target value of the bill must be equal to the sum of the target values of succeeded payments in swap transaction")
	ErrSwapInsufficientDCMoneySupply = goerrors.New("insufficient DC-money supply")
	ErrSwapBillAlreadyExists         = goerrors.New("swapped bill id already exists")
	ErrSwapInvalidBillIdentifiers    = goerrors.New("all bill identifiers in dust transfer orders must exist in transaction bill identifiers")
	ErrSwapInvalidBillId             = goerrors.New("bill id is not properly computed")
	ErrSwapDustTransfersInvalidOrder = goerrors.New("transfer orders are not listed in strictly increasing order of bill identifiers")
	ErrSwapInvalidNonce              = goerrors.New("dust transfer orders do not contain proper nonce")
	ErrSwapInvalidTargetBearer       = goerrors.New("dust transfer orders do not contain proper target bearer")
	ErrInvalidProofType              = goerrors.New("invalid proof type")
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

	if tx.Amount() == 0 {
		return ErrSplitBillZeroAmount
	}
	if tx.RemainingValue() == 0 {
		return ErrSplitBillZeroRemainder
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

func validateSwap(tx Swap, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) error {
	// 1. ExtrType(ι) = bill - target unit is a bill
	// TODO: AB-421
	// 2. target value of the bill is the sum of the target values of succeeded payments in P
	expectedSum := tx.TargetValue()
	actualSum := sumDcTransferValues(tx)
	if expectedSum != actualSum {
		return ErrSwapInvalidTargetValue
	}

	// 3. there is suffiecient DC-money supply
	// 4. there exists no bill with identifier
	// checked in moneyTxSystem#validateSwap method

	// 5. all bill ids in dust transfer orders are elements of bill ids in swap tx
	for _, dcTx := range tx.DCTransfers() {
		exists := billIdInList(dcTx.UnitID(), tx.BillIdentifiers())
		if !exists {
			return ErrSwapInvalidBillIdentifiers
		}
	}

	// 6. new bill id is properly computed ι=h(ι1,...,ιm)
	expectedBillId := hashBillIds(tx, hashAlgorithm)
	unitIdBytes := tx.UnitID().Bytes32()
	if !bytes.Equal(unitIdBytes[:], expectedBillId) {
		return ErrSwapInvalidBillId
	}

	// 7. bills were transfered to DC (validate dc transfer type)
	// already checked on language/protobuf level

	// 8. bill transfer orders are listed in strictly increasing order of bill identifiers
	// (in particular, this ensures that no bill can be included multiple times)
	// 9. bill transfer orders contain proper nonce
	// 10. bill transfer orders contain proper target bearer
	// 11. block proofs of the bill transfer orders verify
	var prevDcTx TransferDC
	dustTransfers := tx.DCTransfers()
	proofs := tx.Proofs()
	if len(dustTransfers) != len(proofs) {
		return errors.Errorf("invalid count of proofs: expected %v, got %v", len(dustTransfers), len(proofs))
	}
	for i, dcTx := range dustTransfers {
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
		proof := proofs[i]
		if proof.ProofType != block.ProofType_PRIM {
			return ErrInvalidProofType
		}
		// verify proof itself
		err := proof.Verify(util.Uint256ToBytes(dcTx.UnitID()), dcTx, trustBase, hashAlgorithm)
		if err != nil {
			return errors.Wrap(err, "proof is not valid")
		}

		prevDcTx = dcTx
	}
	// done in validateGenericTransaction function
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
