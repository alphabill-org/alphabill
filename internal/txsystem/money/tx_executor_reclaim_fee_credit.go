package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
)

var (
	ErrReclaimFCInvalidTargetUnit = errors.New("invalid target unit")
	ErrReclaimFCInvalidTxFee      = errors.New("the transaction fees cannot exceed the transferred value")
	ErrReclaimFCInvalidNonce      = errors.New("invalid nonce")
)

func handleReclaimFeeCreditTx(state *rma.Tree, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier, feeCreditTxRecorder *feeCreditTxRecorder, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[*transactions.ReclaimFeeCreditWrapper] {
	return func(tx *transactions.ReclaimFeeCreditWrapper, currentBlockNumber uint64) error {
		bd, _ := state.GetUnit(tx.UnitID())
		log.Debug("Processing reclaimFC %v", tx.Transaction.ToLogString(log))
		if bd == nil {
			return errors.New("reclaimFC: unit not found")
		}
		bdd, ok := bd.Data.(*BillData)
		if !ok {
			return errors.New("reclaimFC: invalid unit type")
		}

		if err := validateReclaimFC(tx, bdd, trustBase, hashAlgorithm); err != nil {
			return fmt.Errorf("reclaimFC: validation failed: %w", err)
		}

		// calculate actual tx fee cost
		fee := feeCalc()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		// add reclaimed value to source unit
		v := tx.CloseFCTransfer.CloseFC.Amount - tx.CloseFCTransfer.Transaction.ServerMetadata.Fee - fee
		updateFunc := func(data rma.UnitData) (newData rma.UnitData) {
			newBillData, ok := data.(*BillData)
			if !ok {
				return data // TODO should return error instead
			}
			newBillData.V += v
			newBillData.T = currentBlockNumber
			newBillData.Backlink = tx.Hash(hashAlgorithm)
			return newBillData
		}
		updateAction := rma.UpdateData(tx.UnitID(), updateFunc, tx.Hash(hashAlgorithm))

		if err := state.AtomicUpdate(updateAction); err != nil {
			return fmt.Errorf("reclaimFC: failed to update state: %w", err)
		}
		feeCreditTxRecorder.recordReclaimFC(tx)
		return nil
	}
}

func validateReclaimFC(tx *transactions.ReclaimFeeCreditWrapper, bd *BillData, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
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
	if !bytes.Equal(tx.Transaction.UnitId, tx.CloseFCTransfer.CloseFC.TargetUnitId) {
		return ErrReclaimFCInvalidTargetUnit
	}
	if !bytes.Equal(bd.Backlink, tx.CloseFCTransfer.CloseFC.Nonce) {
		return ErrReclaimFCInvalidNonce
	}
	if !bytes.Equal(bd.Backlink, tx.ReclaimFC.Backlink) {
		return ErrInvalidBacklink
	}
	if tx.CloseFCTransfer.Transaction.ServerMetadata.Fee+tx.Transaction.ClientMetadata.MaxFee > tx.CloseFCTransfer.CloseFC.Amount {
		return ErrReclaimFCInvalidTxFee
	}
	// verify proof
	proof := tx.ReclaimFC.CloseFeeCreditProof
	if proof.ProofType != block.ProofType_PRIM {
		return ErrInvalidProofType
	}
	err := proof.Verify(tx.ReclaimFC.CloseFeeCreditTransfer.UnitId, tx.CloseFCTransfer, verifiers, hashAlgorithm)
	if err != nil {
		return fmt.Errorf("invalid proof: %w", err)
	}
	return nil
}
