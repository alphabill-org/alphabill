package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrInvalidDataType  = errors.New("invalid data type")
	ErrInvalidBillValue = errors.New("transaction value must be equal to bill value")
)

func (m *Module) executeTransferTx(tx *types.TransactionOrder, attr *money.TransferAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// update state
	updateDataFunc := updateBillDataFunc(tx, exeCtx.CurrentRound())
	setOwnerFunc := state.SetOwner(tx.UnitID(), attr.NewBearer)
	if err := m.state.Apply(
		setOwnerFunc,
		updateDataFunc,
	); err != nil {
		return nil, fmt.Errorf("transfer: failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) validateTransferTx(tx *types.TransactionOrder, attr *money.TransferAttributes, exeCtx txtypes.ExecutionContext) error {
	unit, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		return fmt.Errorf("transfer validation error: %w", err)
	}
	if err = validateTransfer(unit.Data(), attr); err != nil {
		return fmt.Errorf("transfer validation error: %w", err)
	}
	if err = m.execPredicate(unit.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	return nil
}

func validateTransfer(data types.UnitData, attr *money.TransferAttributes) error {
	return validateAnyTransfer(data, attr.Counter, attr.TargetValue)
}

func validateAnyTransfer(data types.UnitData, counter uint64, targetValue uint64) error {
	bd, ok := data.(*money.BillData)
	if !ok {
		return ErrInvalidDataType
	}
	if bd.IsLocked() {
		return ErrBillLocked
	}
	if bd.Counter != counter {
		return ErrInvalidCounter
	}
	if targetValue != bd.V {
		return ErrInvalidBillValue
	}
	return nil
}

func updateBillDataFunc(tx *types.TransactionOrder, currentBlockNumber uint64) state.Action {
	unitID := tx.UnitID()
	return state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			bd.T = currentBlockNumber
			bd.Counter += 1
			return bd, nil
		})
}
