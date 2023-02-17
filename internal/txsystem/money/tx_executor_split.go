package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
)

var (
	ErrInvalidBillValue                 = errors.New("transaction value must be equal to bill value")
	ErrSplitTxInvalidBillRemainingValue = errors.New("transaction remaining value must equal to the previous bill value minus the transaction amount")
	ErrInvalidDataType                  = errors.New("invalid data type")
)

func handleSplitTx(state *rma.Tree, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[*billSplitWrapper] {
	return func(tx *billSplitWrapper, currentBlockNumber uint64) error {
		log.Debug("Processing split %v", tx)
		if err := validateSplitTx(tx, state); err != nil {
			return fmt.Errorf("invalid split transaction: %w", err)
		}
		h := tx.Hash(hashAlgorithm)
		newItemId := txutil.SameShardID(tx.UnitID(), tx.HashForIdCalculation(hashAlgorithm))

		// calculate actual tx fee cost
		fee := feeCalc()
		tx.transaction.ServerMetadata = &txsystem.ServerMetadata{Fee: fee}

		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.UpdateData(tx.UnitID(),
				func(data rma.UnitData) (newData rma.UnitData) {
					bd, ok := data.(*BillData)
					if !ok {
						return data // todo return error
					}
					return &BillData{
						V:        bd.V - tx.Amount(),
						T:        currentBlockNumber,
						Backlink: tx.Hash(hashAlgorithm),
					}
				}, h),
			rma.AddItem(newItemId, tx.TargetBearer(), &BillData{
				V:        tx.Amount(),
				T:        currentBlockNumber,
				Backlink: h,
			}, h))
	}
}

func validateSplitTx(tx *billSplitWrapper, state *rma.Tree) error {
	data, err := state.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateSplit(data.Data, tx)
}

func validateSplit(data rma.UnitData, tx *billSplitWrapper) error {
	bd, ok := data.(*BillData)
	if !ok {
		return ErrInvalidDataType
	}
	if !bytes.Equal(tx.Backlink(), bd.Backlink) {
		return ErrInvalidBacklink
	}
	// amount does not exceed value of the bill
	if tx.Amount() >= bd.V {
		return ErrInvalidBillValue
	}
	// remaining value equals the previous value minus the amount
	if tx.RemainingValue() != bd.V-tx.Amount() {
		return ErrSplitTxInvalidBillRemainingValue
	}
	return nil
}
