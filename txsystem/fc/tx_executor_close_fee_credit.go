package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
)

func handleCloseFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[fc.CloseFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *fc.CloseFeeCreditAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		bd, _ := f.state.GetUnit(tx.UnitID(), false)
		if err := f.txValidator.ValidateCloseFC(&CloseFCValidationContext{Tx: tx, Unit: bd}); err != nil {
			return nil, fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		decrCreditFn := unit.DecrCredit(tx.UnitID(), attr.Amount)
		updateDataFn := state.UpdateUnitData(tx.UnitID(),
			func(data types.UnitData) (types.UnitData, error) {
				fcr, ok := data.(*fc.FeeCreditRecord)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fee credit record", tx.UnitID())
				}
				fcr.Backlink = tx.Hash(f.hashAlgorithm)
				return fcr, nil
			})
		if err := f.state.Apply(decrCreditFn, updateDataFn); err != nil {
			return nil, fmt.Errorf("closeFC: state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: f.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}
