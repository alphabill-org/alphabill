package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *Module) executeTransferDCTx(tx *types.TransactionOrder, attr *money.TransferDCAttributes, _ *money.TransferDCAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()

	// 1. N[T.ι].D.v ← 0 – wipe out the bill value
	// 2. N[T.ι].D.φ ← DC – mark the bill as collected
	// 3. N[T.ι].D.c ← N[T.ι].D.c + 1 – increment counter
	updateUnitFn := state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			bd.Value = 0
			bd.OwnerPredicate = DustCollectorPredicate
			bd.Counter += 1
			return bd, nil
		},
	)

	// 4. N[T.ιDC].D.v ← N[T.ιDC].D.v + T.A.v – increase the DC money supply by bill value
	updateDCMoneySupplyFn := state.UpdateUnitData(DustCollectorMoneySupplyID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", DustCollectorMoneySupplyID)
			}
			bd.Value += attr.Value
			return bd, nil
		},
	)

	if err := m.state.Apply(
		updateUnitFn,
		updateDCMoneySupplyFn,
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
	if err := m.validateTransDC(tx, attr, authProof, exeCtx); err != nil {
		return fmt.Errorf("validateTransferDC error: %w", err)
	}
	return nil
}

func (m *Module) validateTransDC(tx *types.TransactionOrder, attr *money.TransferDCAttributes, authProof *money.TransferDCAuthProof, exeCtx txtypes.ExecutionContext) error {
	if tx.HasStateLock() {
		return errors.New("transDC transaction cannot contain state lock")
	}
	unit, err := m.state.GetUnit(tx.UnitID, false)
	if err != nil {
		return err
	}
	unitData, ok := unit.Data().(*money.BillData)
	if !ok {
		return errors.New("invalid unit data type")
	}
	if unitData.Counter != attr.Counter {
		return errors.New("the transaction counter is not equal to the bill counter")
	}
	if unitData.Value != attr.Value {
		return errors.New("the transaction value is not equal to the bill value")
	}
	if err = m.execPredicate(unitData.OwnerPredicate, authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	return nil
}
