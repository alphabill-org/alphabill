package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
)

var (
	ErrReclaimFCInvalidTargetUnit = errors.New("invalid target unit")
	ErrReclaimFCInvalidTxFee      = errors.New("the transaction fees cannot exceed the transferred value")
	ErrReclaimFCInvalidNonce      = errors.New("invalid nonce")
)

func handleReclaimFeeCreditTx(s *state.State, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier, feeCreditTxRecorder *feeCreditTxRecorder, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[transactions.ReclaimFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.ReclaimFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()
		bd, _ := s.GetUnit(unitID, false)
		log.Debug("Processing reclaimFC %v", tx)
		if bd == nil {
			return nil, errors.New("reclaimFC: unit not found")
		}
		bdd, ok := bd.Data().(*BillData)
		if !ok {
			return nil, errors.New("reclaimFC: invalid unit type")
		}

		if err := validateReclaimFC(tx, attr, bdd, trustBase, hashAlgorithm); err != nil {
			return nil, fmt.Errorf("reclaimFC: validation failed: %w", err)
		}

		// calculate actual tx fee cost
		fee := feeCalc()
		sm := &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}}
		txr := &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: sm}
		closeFCAttr := &transactions.CloseFeeCreditAttributes{}
		closeFeeCreditTransfer := attr.CloseFeeCreditTransfer
		if err := closeFeeCreditTransfer.TransactionOrder.UnmarshalAttributes(closeFCAttr); err != nil {
			return nil, fmt.Errorf("reclaimFC: failed to unmarshal close fee credit attributes: %w", err)
		}

		// add reclaimed value to source unit
		v := closeFCAttr.Amount - closeFeeCreditTransfer.ServerMetadata.ActualFee - fee
		updateFunc := func(data state.UnitData) (newData state.UnitData) {
			newBillData, ok := data.(*BillData)
			if !ok {
				return data // TODO should return error instead
			}
			newBillData.V += v
			newBillData.T = currentBlockNumber
			newBillData.Backlink = txr.Hash(hashAlgorithm)
			return newBillData
		}
		updateAction := state.UpdateUnitData(unitID, updateFunc)

		if err := s.Apply(updateAction); err != nil {
			return nil, fmt.Errorf("reclaimFC: failed to update state: %w", err)
		}
		feeCreditTxRecorder.recordReclaimFC(
			&reclaimFeeCreditTx{
				tx: tx, attr: attr, closeFCTransferAttr: closeFCAttr, reclaimFee: fee, closeFee: closeFeeCreditTransfer.ServerMetadata.ActualFee})

		return sm, nil
	}
}

func validateReclaimFC(tx *types.TransactionOrder, attr *transactions.ReclaimFeeCreditAttributes, bd *BillData, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	if tx == nil {
		return ErrTxNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if tx.GetClientFeeCreditRecordID() != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}

	closeFeeCreditTx := attr.CloseFeeCreditTransfer
	closeFCAttr := &transactions.CloseFeeCreditAttributes{}
	if err := closeFeeCreditTx.TransactionOrder.UnmarshalAttributes(closeFCAttr); err != nil {
		return fmt.Errorf("invalid close fee credit attributes: %w", err)
	}

	if !bytes.Equal(tx.UnitID(), closeFCAttr.TargetUnitID) {
		return ErrReclaimFCInvalidTargetUnit
	}
	if !bytes.Equal(bd.Backlink, closeFCAttr.Nonce) {
		return ErrReclaimFCInvalidNonce
	}
	if !bytes.Equal(bd.Backlink, attr.Backlink) {
		return ErrInvalidBacklink
	}
	//
	if closeFeeCreditTx.ServerMetadata.ActualFee+tx.Payload.ClientMetadata.MaxTransactionFee > closeFCAttr.Amount {
		return ErrReclaimFCInvalidTxFee
	}
	// verify proof
	if err := types.VerifyTxProof(attr.CloseFeeCreditProof, attr.CloseFeeCreditTransfer, verifiers, hashAlgorithm); err != nil {
		return fmt.Errorf("invalid proof: %w", err)
	}
	return nil
}
