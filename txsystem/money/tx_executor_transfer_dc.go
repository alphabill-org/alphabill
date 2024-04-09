package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *Module) handleTransferDCTx() txsystem.GenericExecuteFunc[TransferDCAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferDCAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateTransferDCTx(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid transferDC tx: %w", err)
		}
		unitID := tx.UnitID()

		// 1. SetOwner(ι, DC)
		setOwnerFn := state.SetOwner(unitID, DustCollectorPredicate)

		// 2. UpdateData(ι0, f′), where f′ : D.v → D.v + N[ι].D.v – increase DC money supply by N[ι].D.v
		updateDCMoneySupplyFn := state.UpdateUnitData(DustCollectorMoneySupplyID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", DustCollectorMoneySupplyID)
				}
				bd.V += attr.Value
				bd.Counter += 1
				return bd, nil
			})

		// 3. UpdateData(ι, f), where f(D) = (0, S.n, H(P))
		updateUnitFn := state.UpdateUnitData(unitID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
				}
				bd.V = 0
				bd.T = exeCtx.CurrentBlockNr
				bd.Counter += 1
				return bd, nil
			})

		if err := m.state.Apply(
			setOwnerFn,
			updateDCMoneySupplyFn,
			updateUnitFn,
		); err != nil {
			return nil, fmt.Errorf("transferDC: failed to update state: %w", err)
		}

		// record dust bills for later deletion TODO AB-1133
		// dustCollector.AddDustBill(unitID, currentBlockNumber)
		return &types.ServerMetadata{
			ActualFee:        m.feeCalculator(),
			TargetUnits:      []types.UnitID{unitID, DustCollectorMoneySupplyID},
			SuccessIndicator: types.TxStatusSuccessful,
		}, nil
	}
}

func (m *Module) validateTransferDCTx(tx *types.TransactionOrder, attr *TransferDCAttributes) error {
	unit, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	if err := m.execPredicate(unit.Bearer(), tx.OwnerProof, tx); err != nil {
		return err
	}
	return validateTransferDC(unit.Data(), attr)
}

func validateTransferDC(data state.UnitData, tx *TransferDCAttributes) error {
	return validateAnyTransfer(data, tx.Counter, tx.Value)
}
