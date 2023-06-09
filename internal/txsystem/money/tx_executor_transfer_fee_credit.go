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
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrTxNil          = errors.New("tx is nil")
	ErrBillNil        = errors.New("bill is nil")
	ErrRecordIDExists = errors.New("fee tx cannot contain fee credit reference")
	ErrFeeProofExists = errors.New("fee tx cannot contain fee authorization proof")

	ErrInvalidFCValue  = errors.New("the amount to transfer plus transaction fee cannot exceed the value of the bill")
	ErrInvalidBacklink = errors.New("the transaction backlink is not equal to unit backlink")
)

func handleTransferFeeCreditTx(state *rma.Tree, hashAlgorithm crypto.Hash, feeCreditTxRecorder *feeCreditTxRecorder, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[transactions.TransferFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.TransferFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing transferFC %v", tx)
		unitID := util.BytesToUint256(tx.UnitID())
		unit, _ := state.GetUnit(unitID)
		if unit == nil {
			return nil, fmt.Errorf("transferFC: unit not found %X", tx.UnitID())
		}
		billData, ok := unit.Data.(*BillData)
		if !ok {
			return nil, errors.New("transferFC: invalid unit type")
		}
		if err := validateTransferFC(tx, attr, billData); err != nil {
			return nil, fmt.Errorf("transferFC: validation failed: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()

		// remove value from source unit, or delete source bill entirely
		var action rma.Action
		v := attr.Amount + fee
		if v < billData.V {
			action = rma.UpdateData(unitID, func(data rma.UnitData) (newData rma.UnitData) {
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
			action = rma.DeleteItem(unitID)
		}
		if err := state.AtomicUpdate(action); err != nil {
			return nil, fmt.Errorf("transferFC: failed to update state: %w", err)
		}
		// record fee tx for end of the round consolidation
		feeCreditTxRecorder.recordTransferFC(&transferFeeCreditTx{
			tx:   tx,
			attr: attr,
			fee:  fee,
		})
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateTransferFC(tx *types.TransactionOrder, attr *transactions.TransferFeeCreditAttributes, bd *BillData) error {
	if tx == nil {
		return ErrTxNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if attr.Amount+tx.Payload.ClientMetadata.MaxTransactionFee > bd.V {
		return ErrInvalidFCValue
	}
	if !bytes.Equal(attr.Backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	if tx.GetClientFeeCreditRecordID() != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}
	return nil
}
