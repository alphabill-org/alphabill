package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
)

func handleCloseFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[transactions.CloseFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.CloseFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		bd, _ := f.state.GetUnit(tx.UnitID(), false)
		if err := f.txValidator.ValidateCloseFC(&CloseFCValidationContext{Tx: tx, Unit: bd}); err != nil {
			return nil, fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		// decrement credit
		if err := f.state.Apply(unit.DecrCredit(tx.UnitID(), attr.Amount)); err != nil {
			return nil, fmt.Errorf("closeFC: state update failed: %w", err)
		}
		// calculate actual tx fee cost
		return &types.ServerMetadata{ActualFee: f.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}
