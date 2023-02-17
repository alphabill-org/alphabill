package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
)

var (
	ErrTxNil          = errors.New("tx is nil")
	ErrBillNil        = errors.New("bill is nil")
	ErrRecordIDExists = errors.New("fee tx cannot contain fee credit reference")
	ErrFeeProofExists = errors.New("fee tx cannot contain fee authorization proof")

	ErrInvalidFCValue  = errors.New("the amount to transfer plus transaction fee cannot exceed the value of the bill")
	ErrInvalidBacklink = errors.New("the transaction backlink is not equal to unit backlink")
)

func handleTransferFeeCreditTx(state *rma.Tree, hashAlgorithm crypto.Hash, feeCreditTxRecorder *feeCreditTxRecorder, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[*transactions.TransferFeeCreditWrapper] {
	return func(tx *transactions.TransferFeeCreditWrapper, currentBlockNumber uint64) error {
		log.Debug("Processing transferFC %v", tx)
		unit, _ := state.GetUnit(tx.UnitID())
		if unit == nil {
			return errors.New("transferFC: unit not found")
		}
		billData, ok := unit.Data.(*BillData)
		if !ok {
			return errors.New("transferFC: invalid unit type")
		}

		if err := validateTransferFC(tx, billData); err != nil {
			return fmt.Errorf("transferFC: validation failed: %w", err)
		}

		// calculate actual tx fee cost
		tx.Wrapper.Transaction.ServerMetadata = &txsystem.ServerMetadata{Fee: feeCalc()}

		// remove value from source unit, or delete source bill entirely
		var action rma.Action
		v := tx.TransferFC.Amount + tx.Wrapper.Transaction.ServerMetadata.Fee
		if v < billData.V {
			action = rma.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
				newBillData, ok := data.(*BillData)
				if !ok {
					return data // TODO should return error instead
				}
				newBillData.V -= v
				newBillData.T = currentBlockNumber
				newBillData.Backlink = tx.Hash(hashAlgorithm)
				return newBillData
			}, tx.Hash(hashAlgorithm))
		} else {
			action = rma.DeleteItem(tx.UnitID())
		}
		if err := state.AtomicUpdate(action); err != nil {
			return fmt.Errorf("transferFC: failed to update state: %w", err)
		}
		// record fee tx for end of the round consolidation
		feeCreditTxRecorder.recordTransferFC(tx)
		return nil
	}
}

func validateTransferFC(tx *transactions.TransferFeeCreditWrapper, bd *BillData) error {
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
