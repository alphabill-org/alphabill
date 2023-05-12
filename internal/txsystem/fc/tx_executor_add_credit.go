package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
)

func handleAddFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[*transactions.AddFeeCreditWrapper] {
	return func(tx *transactions.AddFeeCreditWrapper, currentBlockNumber uint64) error {
		f.logger.Debug("Processing addFC %v", tx.Transaction.ToLogString(f.logger))
		bd, _ := f.state.GetUnit(tx.UnitID())
		if err := f.txValidator.ValidateAddFeeCredit(&AddFCValidationContext{
			Tx:                 tx,
			Unit:               bd,
			CurrentRoundNumber: currentBlockNumber,
		}); err != nil {
			return fmt.Errorf("addFC tx validation failed: %w", err)
		}
		// calculate actual tx fee cost
		fee := f.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		var updateFunc rma.Action
		// find net value of credit
		v := tx.TransferFC.TransferFC.Amount - fee
		if bd == nil {
			// add credit
			fcr := &FeeCreditRecord{
				Balance: v,
				Hash:    tx.Hash(f.hashAlgorithm),
				Timeout: tx.TransferFC.TransferFC.LatestAdditionTime + 1,
			}
			updateFunc = AddCredit(tx.UnitID(), tx.AddFC.FeeCreditOwnerCondition, fcr, tx.Hash(f.hashAlgorithm))
		} else {
			// increment credit
			updateFunc = IncrCredit(tx.UnitID(), v, tx.TransferFC.TransferFC.LatestAdditionTime+1, tx.Hash(f.hashAlgorithm))
		}

		if err := f.state.AtomicUpdate(updateFunc); err != nil {
			return fmt.Errorf("addFC state update failed: %w", err)
		}
		return nil
	}
}
