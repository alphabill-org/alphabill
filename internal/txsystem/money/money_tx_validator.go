package money

import (
	"bytes"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
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
	ErrInvalidProofType              = errors.New("invalid proof type")
	ErrSwapOwnerProofFailed          = errors.New("owner proof does not satisfy the bearer condition of the swapped bill")

	// fee tx generic errors
	ErrTxNil          = errors.New("tx is nil")
	ErrBillNil        = errors.New("bill is nil")
	ErrRecordIDExists = errors.New("fee tx cannot contain fee credit reference")
	ErrFeeProofExists = errors.New("fee tx cannot contain fee authorization proof")

	// transfer fee credit errors
	ErrInvalidFCValue  = errors.New("the amount to transfer plus transaction fee cannot exceed the value of the bill")
	ErrInvalidBacklink = errors.New("the transaction backlink is not equal to unit backlink")

	// reclaim fee credit errors
	ErrReclFCInvalidCloseFCType = errors.New("reclaimFC: invalid nested closeFC tx type")
	ErrReclFCInvalidTargetUnit  = errors.New("reclaimFC: invalid target unit")
	ErrReclFCInvalidTxFee       = errors.New("reclaimFC: the transaction fees cannot exceed the transferred value")
	ErrReclFCInvalidNonce       = errors.New("reclaimFC: invalid nonce")
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

	// 7. transfers were in the money partition
	// 8. bills were transferred to DC (validate dc transfer type)
	// already checked on language/protobuf level

	var prevDcTx TransferDC
	dustTransfers := tx.DCTransfers()
	proofs := tx.Proofs()
	if len(dustTransfers) != len(proofs) {
		return errors.Errorf("invalid count of proofs: expected %v, got %v", len(dustTransfers), len(proofs))
	}
	for i, dcTx := range dustTransfers {
		// 9. bill transfer orders are listed in strictly increasing order of bill identifiers
		// (in particular, this ensures that no bill can be included multiple times)
		if (i > 0) && !dcTx.UnitID().Gt(prevDcTx.UnitID()) {
			return ErrSwapDustTransfersInvalidOrder
		}

		err := validateDustTransfer(dcTx, proofs[i], unitIdBytes, tx.OwnerCondition(), hashAlgorithm, trustBase)
		if err != nil {
			return err
		}

		prevDcTx = dcTx
	}

	// 12. the owner proof of the swap transaction satisfies the bearer condition of the new bill
	err := script.RunScript(tx.OwnerProof(), tx.OwnerCondition(), tx.SigBytes())
	if err != nil {
		return errors.Wrap(err, ErrSwapOwnerProofFailed.Error())
	}
	return nil
}

func validateDustTransfer(dcTx TransferDC, proof *block.BlockProof, unitIdBytes [32]byte, ownerCondition []byte, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) error {
	// 10. bill transfer orders contain proper nonce
	if !bytes.Equal(dcTx.Nonce(), unitIdBytes[:]) {
		return ErrSwapInvalidNonce
	}
	// 11. bill transfer orders contain proper target bearer
	if !bytes.Equal(dcTx.TargetBearer(), ownerCondition) {
		return ErrSwapInvalidTargetBearer
	}
	// 13. block proofs of the bill transfer orders verify
	if proof.ProofType != block.ProofType_PRIM {
		return ErrInvalidProofType
	}
	err := proof.Verify(util.Uint256ToBytes(dcTx.UnitID()), dcTx, trustBase, hashAlgorithm)
	if err != nil {
		return errors.Wrap(err, "proof is not valid")
	}

	return nil
}

func validateTransferFC(tx *fc.TransferFeeCreditWrapper, bd *BillData) error {
	if tx == nil {
		return ErrTxNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if tx.TransferFC.Amount+tx.Transaction.ClientMetadata.MaxFee > bd.V {
		return ErrInvalidFCValue
	}
	if !bytes.Equal(tx.TransferFC.Backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	if tx.Transaction.ClientMetadata.FeeCreditRecordId != nil {
		return ErrRecordIDExists
	}
	if tx.Transaction.FeeProof != nil {
		return ErrFeeProofExists
	}
	return nil
}

func validateReclaimFC(tx *fc.ReclaimFeeCreditWrapper, bd *BillData, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	if tx == nil {
		return ErrTxNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if tx.Transaction.ClientMetadata.FeeCreditRecordId != nil {
		return ErrRecordIDExists
	}
	if tx.Transaction.FeeProof != nil {
		return ErrFeeProofExists
	}
	closeFC, err := fc.NewFeeCreditTx(tx.ReclaimFC.CloseFeeCreditTransfer)
	if err != nil {
		return err
	}
	closeFCWrapper, ok := closeFC.(*fc.CloseFeeCreditWrapper)
	if !ok {
		return ErrReclFCInvalidCloseFCType
	}
	if !bytes.Equal(tx.Transaction.UnitId, closeFCWrapper.CloseFC.TargetUnitId) {
		return ErrReclFCInvalidTargetUnit
	}
	if closeFCWrapper.Transaction.ServerMetadata.Fee+tx.Transaction.ClientMetadata.MaxFee > bd.V {
		return ErrReclFCInvalidTxFee
	}
	if !bytes.Equal(bd.Backlink, closeFCWrapper.CloseFC.Nonce) {
		return ErrReclFCInvalidNonce
	}
	if !bytes.Equal(bd.Backlink, tx.ReclaimFC.Backlink) {
		return ErrInvalidBacklink
	}
	// verify proof
	proof := tx.ReclaimFC.CloseFeeCreditProof
	if proof.ProofType != block.ProofType_PRIM {
		return ErrInvalidProofType
	}
	err = proof.Verify(tx.ReclaimFC.CloseFeeCreditTransfer.UnitId, closeFC, verifiers, hashAlgorithm)
	if err != nil {
		return errors.Wrap(err, "reclaimFC: invalid proof")
	}
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
