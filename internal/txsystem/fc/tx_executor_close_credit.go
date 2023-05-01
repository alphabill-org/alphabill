package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
)

func handleCloseFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[*transactions.CloseFeeCreditWrapper] {
	return func(tx *transactions.CloseFeeCreditWrapper, currentBlockNumber uint64) error {
		f.logger.Debug("Processing closeFC %v", tx)
		bd, _ := f.state.GetUnit(tx.UnitID())
		if err := f.txValidator.ValidateCloseFC(&CloseFCValidationContext{Tx: tx, Unit: bd}); err != nil {
			return fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		// calculate actual tx fee cost
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: f.feeCalculator()})

		// decrement credit
		updateFunc := DecrCredit(tx.UnitID(), tx.CloseFC.Amount, tx.Hash(f.hashAlgorithm))
		if err := f.state.AtomicUpdate(updateFunc); err != nil {
			return fmt.Errorf("closeFC: state update failed: %w", err)
		}
		return nil
	}
}
