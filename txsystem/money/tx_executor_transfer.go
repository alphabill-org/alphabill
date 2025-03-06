package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *Module) executeTransferTx(tx *types.TransactionOrder, attr *money.TransferAttributes, _ *money.TransferAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// update state
	dataUpdateFn := state.UpdateUnitData(tx.UnitID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", tx.UnitID)
			}
			bd.Counter += 1
			bd.OwnerPredicate = attr.NewOwnerPredicate
			return bd, nil
		},
	)
	if err := m.state.Apply(
		dataUpdateFn,
	); err != nil {
		return nil, fmt.Errorf("transfer: failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{tx.UnitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) validateTransferTx(tx *types.TransactionOrder, attr *money.TransferAttributes, authProof *money.TransferAuthProof, exeCtx txtypes.ExecutionContext) error {
	if err := m.validateTrans(tx, attr, authProof, exeCtx); err != nil {
		return fmt.Errorf("transfer validation error: %w", err)
	}
	return nil
}

func (m *Module) validateTrans(tx *types.TransactionOrder, attr *money.TransferAttributes, authProof *money.TransferAuthProof, exeCtx txtypes.ExecutionContext) error {
	unit, err := m.state.GetUnit(tx.UnitID, false)
	if err != nil {
		return err
	}
	unitData, ok := unit.Data().(*money.BillData)
	if !ok {
		return errors.New("invalid unit data type")
	}
	if unitData.IsLocked() {
		return errors.New("the bill is locked")
	}
	if unitData.Counter != attr.Counter {
		return errors.New("the transaction counter is not equal to the bill counter")
	}
	if unitData.Value != attr.TargetValue {
		return errors.New("the transaction value is not equal to the bill value")
	}
	if err = m.execPredicate(unitData.OwnerPredicate, authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	return nil
}
