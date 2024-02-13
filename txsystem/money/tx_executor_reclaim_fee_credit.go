package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/types"
)

var (
	ErrReclaimFCInvalidTargetUnit         = errors.New("invalid target unit")
	ErrReclaimFCInvalidTxFee              = errors.New("the transaction fees cannot exceed the transferred value")
	ErrReclaimFCInvalidTargetUnitBacklink = errors.New("invalid target unit backlink")
)

func handleReclaimFeeCreditTx(s *state.State, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier, feeCreditTxRecorder *feeCreditTxRecorder, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[transactions.ReclaimFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.ReclaimFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()
		unit, _ := s.GetUnit(unitID, false)
		if unit == nil {
			return nil, errors.New("reclaimFC: unit not found")
		}
		if err := txsystem.VerifyUnitOwnerProof(tx, unit.Bearer()); err != nil {
			return nil, err
		}
		bdd, ok := unit.Data().(*BillData)
		if !ok {
			return nil, errors.New("reclaimFC: invalid unit type")
		}

		if err := validateReclaimFC(tx, attr, bdd, trustBase, hashAlgorithm); err != nil {
			return nil, fmt.Errorf("reclaimFC: validation failed: %w", err)
		}

		// calculate actual tx fee cost
		fee := feeCalc()

		closeFCAttr := &transactions.CloseFeeCreditAttributes{}
		closeFeeCreditTransfer := attr.CloseFeeCreditTransfer
		if err := closeFeeCreditTransfer.TransactionOrder.UnmarshalAttributes(closeFCAttr); err != nil {
			return nil, fmt.Errorf("reclaimFC: failed to unmarshal close fee credit attributes: %w", err)
		}

		// add reclaimed value to source unit
		v := closeFCAttr.Amount - closeFeeCreditTransfer.ServerMetadata.ActualFee - fee
		updateFunc := func(data state.UnitData) (state.UnitData, error) {
			newBillData, ok := data.(*BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			newBillData.V += uint64(v)
			newBillData.T = currentBlockNumber
			newBillData.Backlink = tx.Hash(hashAlgorithm)
			newBillData.Locked = 0
			return newBillData, nil
		}
		updateAction := state.UpdateUnitData(unitID, updateFunc)

		if err := s.Apply(updateAction); err != nil {
			return nil, fmt.Errorf("reclaimFC: failed to update state: %w", err)
		}
		feeCreditTxRecorder.recordReclaimFC(
			&reclaimFeeCreditTx{
				tx:                  tx,
				attr:                attr,
				closeFCTransferAttr: closeFCAttr,
				reclaimFee:          fee,
				closeFee:            closeFeeCreditTransfer.ServerMetadata.ActualFee,
			},
		)
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
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
	if !bytes.Equal(bd.Backlink, closeFCAttr.TargetUnitBacklink) {
		return ErrReclaimFCInvalidTargetUnitBacklink
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
