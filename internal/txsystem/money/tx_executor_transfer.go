package money

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

func handleTransferTx(state *rma.Tree, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[*transferWrapper] {
	return func(tx *transferWrapper, currentBlockNumber uint64) error {
		log.Debug("Processing transfer %v", tx)

		if err := validateTransferTx(tx, state); err != nil {
			return fmt.Errorf("invalid transfer tx: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		tx.transaction.ServerMetadata = &txsystem.ServerMetadata{Fee: fee}

		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		decrCreditFunc := fc.DecrCredit(fcrID, fee, tx.Hash(hashAlgorithm))
		updateDataFunc := updateBillDataFunc(tx, currentBlockNumber, hashAlgorithm)
		setOwnerFunc := rma.SetOwner(tx.UnitID(), tx.NewBearer(), tx.Hash(hashAlgorithm))
		if err := state.AtomicUpdate(
			decrCreditFunc,
			updateDataFunc,
			setOwnerFunc,
		); err != nil {
			return fmt.Errorf("transfer: failed to update state: %w", err)
		}
		return nil
	}
}

func validateTransferTx(tx *transferWrapper, state *rma.Tree) error {
	data, err := state.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateTransfer(data.Data, tx)
}

func validateTransfer(data rma.UnitData, tx *transferWrapper) error {
	return validateAnyTransfer(data, tx.Backlink(), tx.TargetValue())
}

func validateAnyTransfer(data rma.UnitData, backlink []byte, targetValue uint64) error {
	bd, ok := data.(*BillData)
	if !ok {
		return ErrInvalidDataType
	}
	if !bytes.Equal(backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	if targetValue != bd.V {
		return ErrInvalidBillValue
	}
	return nil
}

func updateBillDataFunc(tx txsystem.GenericTransaction, currentBlockNumber uint64, hashAlgorithm crypto.Hash) rma.Action {
	return rma.UpdateData(tx.UnitID(),
		func(data rma.UnitData) (newData rma.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				return data // TODO return error
			}
			bd.T = currentBlockNumber
			bd.Backlink = tx.Hash(hashAlgorithm)
			return bd
		}, tx.Hash(hashAlgorithm))
}
