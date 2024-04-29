package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

var (
	ErrInvalidDataType  = errors.New("invalid data type")
	ErrInvalidBillValue = errors.New("transaction value must be equal to bill value")
)

func (m *Module) handleTransferTx() txsystem.GenericExecuteFunc[TransferAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferAttributes, exeCtx *txsystem.TxExecutionContext) (sm *types.ServerMetadata, err error) {
		isLocked := false
		if !exeCtx.StateLockReleased {
			if err = m.validateTransferTx(tx, attr, exeCtx); err != nil {
				return nil, fmt.Errorf("invalid transfer tx: %w", err)
			}

			isLocked, err = txsystem.LockUnitState(tx, m.execPredicate, m.state, exeCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to lock unit state: %w", err)
			}
		}

		// calculate actual tx fee cost
		fee := m.feeCalculator()

		if !isLocked {
			// update state
			updateDataFunc := updateBillDataFunc(tx, exeCtx.CurrentBlockNr)
			setOwnerFunc := state.SetOwner(tx.UnitID(), attr.NewBearer)
			if err := m.state.Apply(
				setOwnerFunc,
				updateDataFunc,
			); err != nil {
				return nil, fmt.Errorf("transfer: failed to update state: %w", err)
			}
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *Module) validateTransferTx(tx *types.TransactionOrder, attr *TransferAttributes, exeCtx *txsystem.TxExecutionContext) error {
	unit, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	if err := m.execPredicate(unit.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	return validateTransfer(unit.Data(), attr)
}

func validateTransfer(data state.UnitData, attr *TransferAttributes) error {
	return validateAnyTransfer(data, attr.Counter, attr.TargetValue)
}

func validateAnyTransfer(data state.UnitData, counter uint64, targetValue uint64) error {
	bd, ok := data.(*BillData)
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
		func(data state.UnitData) (state.UnitData, error) {
			bd, ok := data.(*BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			bd.T = currentBlockNumber
			bd.Counter += 1
			return bd, nil
		})
}
