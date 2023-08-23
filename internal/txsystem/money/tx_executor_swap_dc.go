package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

var (
	ErrSwapInvalidTargetValue        = errors.New("target value of the bill must be equal to the sum of the target values of succeeded payments in swap transaction")
	ErrSwapInsufficientDCMoneySupply = errors.New("insufficient DC-money supply")
	ErrSwapBillAlreadyExists         = errors.New("swapped bill id already exists")
	ErrSwapInvalidBillIdentifiers    = errors.New("all bill identifiers in dust transfer orders must exist in transaction bill identifiers")
	ErrSwapInvalidBillId             = errors.New("bill id is not properly computed")
	ErrSwapDustTransfersInvalidOrder = errors.New("transfer orders are not listed in strictly increasing order of bill identifiers")
	ErrSwapInvalidNonce              = errors.New("dust transfer orders do not contain proper nonce")
	ErrSwapInvalidTargetBearer       = errors.New("dust transfer orders do not contain proper target bearer")
	ErrSwapOwnerProofFailed          = errors.New("owner proof does not satisfy the bearer condition of the swapped bill")
)

type dustCollectorTransfer struct {
	id         *uint256.Int
	tx         *types.TransactionRecord
	attributes *TransferDCAttributes
}

func handleSwapDCTx(s *state.State, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[SwapDCAttributes] {
	return func(tx *types.TransactionOrder, attr *SwapDCAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing swap %v", tx)
		dustTransfers, err := validateSwapTx(tx, attr, s, currentBlockNumber, hashAlgorithm, trustBase)
		if err != nil {
			return nil, fmt.Errorf("invalid swap transaction: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()

		h := tx.Hash(hashAlgorithm)

		// reduce dc-money supply by target value and update timeout and backlink
		updateDCMoneySupplyFn := state.UpdateUnitData(dustCollectorMoneySupplyID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", dustCollectorMoneySupplyID)
				}
				bd.V -= attr.TargetValue
				bd.T = currentBlockNumber
				bd.Backlink = h
				return bd, nil
			},
		)
		// add new unit to state
		addNewUnitFn := state.AddUnit(tx.UnitID(), attr.OwnerCondition, &BillData{
			V:        attr.TargetValue,
			T:        currentBlockNumber,
			Backlink: h,
			TDust:    dustTransfers[0].attributes.SwapTimeout,
		})
		if err := s.Apply(updateDCMoneySupplyFn, addNewUnitFn); err != nil {
			return nil, fmt.Errorf("unit update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateSwapTx(tx *types.TransactionOrder, attr *SwapDCAttributes, s *state.State, currentBlockNumber uint64, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) ([]*dustCollectorTransfer, error) {
	dustTransfers, err := getDCTransfers(attr)
	if err != nil {
		return nil, fmt.Errorf("failed to extract DC transfers: %w", err)
	}
	// 3. there is sufficient DC-money supply
	dcMoneySupply, err := s.GetUnit(dustCollectorMoneySupplyID, false)
	if err != nil {
		return nil, err
	}
	dcMoneySupplyBill, ok := dcMoneySupply.Data().(*BillData)
	if !ok {
		return nil, ErrInvalidDataType
	}
	if dcMoneySupplyBill.V < attr.TargetValue {
		return nil, ErrSwapInsufficientDCMoneySupply
	}
	// 4. there exists no bill with identifier
	if _, err = s.GetUnit(tx.UnitID(), false); err == nil {
		return nil, ErrSwapBillAlreadyExists
	}
	err = validateSwap(tx, attr, dustTransfers, currentBlockNumber, hashAlgorithm, trustBase)
	if err != nil {
		return nil, err
	}
	return dustTransfers, nil
}

func validateSwap(tx *types.TransactionOrder, attr *SwapDCAttributes, dustTransfers []*dustCollectorTransfer, currentBlockNumber uint64, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) error {
	// 1. swap timeout check (all timeouts must be equal to each other and greater than current block)
	err := verifyTimeouts(dustTransfers, currentBlockNumber)
	if err != nil {
		return err
	}
	// 2. target value of the bill is the sum of the target values of succeeded payments in P
	if attr.TargetValue != sumDcTransferValues(dustTransfers) {
		return ErrSwapInvalidTargetValue
	}

	// 3. there is sufficient DC-money supply
	// 4. there exists no bill with identifier
	// checked in moneyTxSystem#validateSwap method

	// 5. all bill ids in dust transfer orders are elements of bill ids in swap tx
	billIdentifiers := toUint256(attr.BillIdentifiers)
	for _, dcTx := range dustTransfers {
		billID := util.BytesToUint256(dcTx.tx.TransactionOrder.UnitID())
		exists := billIdInList(billID, billIdentifiers)
		if !exists {
			return ErrSwapInvalidBillIdentifiers
		}
	}

	// 6. new bill id is properly computed ι=h(ι1,...,ιm)
	expectedBillId := hashBillIds(dustTransfers, hashAlgorithm)
	unitIdBytes := util.BytesToUint256(tx.UnitID()).Bytes32()
	if !bytes.Equal(unitIdBytes[:], expectedBillId) {
		return ErrSwapInvalidBillId
	}

	// 7. transfers were in the money partition
	// 8. bills were transferred to DC (validate dc transfer type)
	proofs := attr.Proofs
	if len(dustTransfers) != len(proofs) {
		return fmt.Errorf("invalid count of proofs: expected %v, got %v", len(dustTransfers), len(proofs))
	}
	var prevDcTx *dustCollectorTransfer
	for i, dcTx := range dustTransfers {
		// 9. bill transfer orders are listed in strictly increasing order of bill identifiers
		// (in particular, this ensures that no bill can be included multiple times)
		if (i > 0) && !dcTx.id.Gt(prevDcTx.id) {
			return ErrSwapDustTransfersInvalidOrder
		}
		if err := validateDustTransfer(dcTx, proofs[i], unitIdBytes, attr.OwnerCondition, hashAlgorithm, trustBase); err != nil {
			return err
		}
		prevDcTx = dcTx
	}

	// 12. the owner proof of the swap transaction satisfies the bearer condition of the new bill
	payloadBytes, err := tx.PayloadBytes()
	if err != nil {
		return fmt.Errorf("failed to get payload bytes: %w", err)
	}
	if err := script.RunScript(tx.OwnerProof, attr.OwnerCondition, payloadBytes); err != nil {
		return ErrSwapOwnerProofFailed
	}
	return nil
}

func verifyTimeouts(dustTransfers []*dustCollectorTransfer, currentBlockNumber uint64) error {
	firstTimeout := dustTransfers[0].attributes.SwapTimeout
	for _, dcTx := range dustTransfers {
		if dcTx.attributes.SwapTimeout <= currentBlockNumber {
			return fmt.Errorf("dust transfer timeout exceeded: tx timeout %d vs current block %d", dcTx.attributes.SwapTimeout, currentBlockNumber)
		}
		if dcTx.attributes.SwapTimeout != firstTimeout {
			return errors.New("dust transfer timeouts are not equal")
		}
	}
	return nil
}

func toUint256(ids [][]byte) []*uint256.Int {
	identifiers := make([]*uint256.Int, len(ids))
	for i, id := range ids {
		identifiers[i] = util.BytesToUint256(id)
	}
	return identifiers
}

func getDCTransfers(attr *SwapDCAttributes) ([]*dustCollectorTransfer, error) {
	if len(attr.DcTransfers) == 0 {
		return nil, errors.New("tx does not contain any dust transfers")
	}
	transfers := make([]*dustCollectorTransfer, len(attr.DcTransfers))
	for i, t := range attr.DcTransfers {
		if t == nil {
			return nil, fmt.Errorf("dc tx is nil: %d", i)
		}
		a := &TransferDCAttributes{}
		if t.TransactionOrder.PayloadType() != PayloadTypeTransDC {
			return nil, fmt.Errorf("invalid transfer DC payload type: %s", t.TransactionOrder.PayloadType())
		}
		if err := t.TransactionOrder.UnmarshalAttributes(a); err != nil {
			return nil, fmt.Errorf("invalid DC transfer: %w", err)
		}
		transfers[i] = &dustCollectorTransfer{
			id:         util.BytesToUint256(t.TransactionOrder.UnitID()),
			tx:         t,
			attributes: a,
		}
	}
	return transfers, nil
}

func validateDustTransfer(dcTx *dustCollectorTransfer, proof *types.TxProof, unitIdBytes [32]byte, ownerCondition []byte, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier) error {
	// 10. bill transfer orders contain proper nonce
	if !bytes.Equal(dcTx.attributes.Nonce, unitIdBytes[:]) {
		return ErrSwapInvalidNonce
	}
	// 11. bill transfer orders contain proper target bearer
	if !bytes.Equal(dcTx.attributes.TargetBearer, ownerCondition) {
		return ErrSwapInvalidTargetBearer
	}
	// 13. block proofs of the bill transfer orders verify
	if err := types.VerifyTxProof(proof, dcTx.tx, trustBase, hashAlgorithm); err != nil {
		return fmt.Errorf("proof is not valid: %w", err)
	}
	return nil
}

func hashBillIds(txs []*dustCollectorTransfer, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	for _, tx := range txs {
		hasher.Write(util.Uint256ToBytes(tx.id))
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

func sumDcTransferValues(txs []*dustCollectorTransfer) uint64 {
	sum := uint64(0)
	for _, dcTx := range txs {
		sum += dcTx.attributes.TargetValue
	}
	return sum
}
