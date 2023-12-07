package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	fcunit "github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
)

func handleUnlockFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[transactions.UnlockFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.UnlockFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()

		bd, _ := f.state.GetUnit(unitID, false)
		if err := f.txValidator.ValidateUnlockFC(&UnlockFCValidationContext{
			Tx:   tx,
			Unit: bd,
			Attr: attr,
		}); err != nil {
			return nil, fmt.Errorf("unlockFC validation failed: %w", err)
		}
		fee := f.feeCalculator()
		txHash := tx.Hash(f.hashAlgorithm)
		updateFunc := state.UpdateUnitData(unitID,
			func(data state.UnitData) (state.UnitData, error) {
				fcr, ok := data.(*fcunit.FeeCreditRecord)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fee credit record", unitID)
				}
				fcr.Balance -= fee
				fcr.Backlink = txHash
				fcr.Locked = 0
				return fcr, nil
			})
		if err := f.state.Apply(updateFunc); err != nil {
			return nil, fmt.Errorf("lockFC state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}
