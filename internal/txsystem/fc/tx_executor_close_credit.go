package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

func handleCloseFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[transactions.CloseFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.CloseFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		f.logger.Debug("Processing closeFC %v", tx)
		bd, _ := f.state.GetUnit(util.BytesToUint256(tx.UnitID()))
		if err := f.txValidator.ValidateCloseFC(&CloseFCValidationContext{Tx: tx, Unit: bd}); err != nil {
			return nil, fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		// calculate actual tx fee cost
		sm := &types.ServerMetadata{ActualFee: f.feeCalculator()}

		// decrement credit
		updateFunc := DecrCredit(util.BytesToUint256(tx.UnitID()), attr.Amount, tx.Hash(f.hashAlgorithm))
		if err := f.state.AtomicUpdate(updateFunc); err != nil {
			return nil, fmt.Errorf("closeFC: state update failed: %w", err)
		}
		return sm, nil
	}
}
