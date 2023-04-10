package money

import (
	"bytes"
	"crypto"
	goerrors "errors"
	"fmt"

	"github.com/holiman/uint256"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrSwapInvalidTargetValue        = goerrors.New("target value of the bill must be equal to the sum of the target values of succeeded payments in swap transaction")
	ErrSwapInsufficientDCMoneySupply = goerrors.New("insufficient DC-money supply")
	ErrSwapBillAlreadyExists         = goerrors.New("swapped bill id already exists")
	ErrSwapInvalidBillIdentifiers    = goerrors.New("all bill identifiers in dust transfer orders must exist in transaction bill identifiers")
	ErrSwapInvalidBillId             = goerrors.New("bill id is not properly computed")
	ErrSwapDustTransfersInvalidOrder = goerrors.New("transfer orders are not listed in strictly increasing order of bill identifiers")
	ErrSwapInvalidNonce              = goerrors.New("dust transfer orders do not contain proper nonce")
	ErrSwapInvalidTargetBearer       = goerrors.New("dust transfer orders do not contain proper target bearer")
	ErrInvalidProofType              = goerrors.New("invalid proof type")
	ErrSwapOwnerProofFailed          = goerrors.New("owner proof does not satisfy the bearer condition of the swapped bill")
)

func handleSwapDCTx(state *rma.Tree, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[*swapDCWrapper] {
	return func(tx *swapDCWrapper, currentBlockNumber uint64) error {
		log.Debug("Processing swap %v", tx)

		if err := validateSwapTx(tx, state, hashAlgorithm, trustBase); err != nil {
			return fmt.Errorf("invalid swap transaction: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		// set n as the target value
		n := tx.TargetValue()
		// reduce dc-money supply by n
		decDustCollectorSupplyFn := func(data rma.UnitData) (newData rma.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				return bd
			}
			bd.V -= n
			return bd
		}
		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, tx.Hash(hashAlgorithm)),
			rma.UpdateData(dustCollectorMoneySupplyID, decDustCollectorSupplyFn, []byte{}),
			rma.AddItem(tx.UnitID(), tx.OwnerCondition(), &BillData{
				V:        n,
				T:        currentBlockNumber,
				Backlink: tx.Hash(hashAlgorithm),
			}, tx.Hash(hashAlgorithm)))
	}
}

func validateSwapTx(tx *swapDCWrapper, state *rma.Tree, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) error {
	// 3. there is sufficient DC-money supply
	dcMoneySupply, err := state.GetUnit(dustCollectorMoneySupplyID)
	if err != nil {
		return err
	}
	dcMoneySupplyBill, ok := dcMoneySupply.Data.(*BillData)
	if !ok {
		return ErrInvalidDataType
	}
	if dcMoneySupplyBill.V < tx.TargetValue() {
		return ErrSwapInsufficientDCMoneySupply
	}
	// 4.there exists no bill with identifier
	if _, err = state.GetUnit(tx.UnitID()); err == nil {
		return ErrSwapBillAlreadyExists
	}
	return validateSwap(tx, hashAlgorithm, trustBase)
}

func validateSwap(tx *swapDCWrapper, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) error {
	// 1. ExtrType(ι) = bill - target unit is a bill
	// TODO: AB-421
	// 2. target value of the bill is the sum of the target values of succeeded payments in P
	if tx.TargetValue() != sumDcTransferValues(tx) {
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
		return fmt.Errorf("invalid count of proofs: expected %v, got %v", len(dustTransfers), len(proofs))
	}
	for i, dcTx := range dustTransfers {
		// 9. bill transfer orders are listed in strictly increasing order of bill identifiers
		// (in particular, this ensures that no bill can be included multiple times)
		if (i > 0) && !dcTx.UnitID().Gt(prevDcTx.UnitID()) {
			return ErrSwapDustTransfersInvalidOrder
		}

		if err := validateDustTransfer(dcTx, proofs[i], unitIdBytes, tx.OwnerCondition(), hashAlgorithm, trustBase); err != nil {
			return err
		}

		prevDcTx = dcTx
	}

	// 12. the owner proof of the swap transaction satisfies the bearer condition of the new bill
	if err := script.RunScript(tx.OwnerProof(), tx.OwnerCondition(), tx.SigBytes()); err != nil {
		return errors.Wrap(err, ErrSwapOwnerProofFailed.Error())
	}
	return nil
}

func validateDustTransfer(dcTx *transferDCWrapper, proof *block.BlockProof, unitIdBytes [32]byte, ownerCondition []byte, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) error {
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

	if err := proof.Verify(util.Uint256ToBytes(dcTx.UnitID()), dcTx, trustBase, hashAlgorithm); err != nil {
		return errors.Wrap(err, "proof is not valid")
	}

	return nil
}

func hashBillIds(tx *swapDCWrapper, hashAlgorithm crypto.Hash) []byte {
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

func sumDcTransferValues(tx *swapDCWrapper) uint64 {
	sum := uint64(0)
	for _, dcTx := range tx.DCTransfers() {
		sum += dcTx.TargetValue()
	}
	return sum
}
