package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

func (m *Module) executeTransferDCTx(tx *types.TransactionOrder, attr *money.TransferDCAttributes, _ *money.TransferDCAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	// 1. SetOwner(ι, DC)
	setOwnerFn := state.SetOwner(unitID, DustCollectorPredicate)

	// 2. UpdateData(ι0, f′), where f′ : D.v → D.v + N[ι].D.v – increase DC money supply by N[ι].D.v
	updateDCMoneySupplyFn := state.UpdateUnitData(DustCollectorMoneySupplyID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", DustCollectorMoneySupplyID)
			}
			bd.V += attr.Value
			bd.Counter += 1
			return bd, nil
		})

	// 3. UpdateData(ι, f), where f(D) = (0, S.n, H(P))
	updateUnitFn := state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			bd.V = 0
			bd.T = exeCtx.CurrentRound()
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
		TargetUnits:      []types.UnitID{unitID, DustCollectorMoneySupplyID},
		SuccessIndicator: types.TxStatusSuccessful,
	}, nil
}

func (m *Module) validateTransferDCTx(tx *types.TransactionOrder, attr *money.TransferDCAttributes, authProof *money.TransferDCAuthProof, exeCtx txtypes.ExecutionContext) error {
	unit, err := m.state.GetUnit(tx.UnitID, false)
	if err != nil {
		return err
	}
	if err = validateTransferDC(unit.Data(), attr); err != nil {
		return fmt.Errorf("validateTransferDC error: %w", err)
	}
	if err = m.execPredicate(unit.Owner(), authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("validateTransferDC error: %w", err)
	}
	return nil
}

func validateTransferDC(data types.UnitData, tx *money.TransferDCAttributes) error {
	return validateAnyTransfer(data, tx.Counter, tx.Value)
}
