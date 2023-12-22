package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
)

func handleCloseFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[transactions.CloseFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.CloseFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		bd, _ := f.state.GetUnit(tx.UnitID(), false)
		if err := f.txValidator.ValidateCloseFC(&CloseFCValidationContext{Tx: tx, Unit: bd}); err != nil {
			return nil, fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		updateDataFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				fcr, ok := data.(unit.GenericFeeCreditRecord)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fee credit record", tx.UnitID())
				}
				fcr.DecCredit(attr.Amount, tx.Hash(f.hashAlgorithm))
				return fcr, nil
			})
		if err := f.state.Apply(updateDataFn); err != nil {
			return nil, fmt.Errorf("closeFC: state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: f.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}
