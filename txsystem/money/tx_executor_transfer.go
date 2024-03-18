package money

import (
	"bytes"
	"crypto"
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
	return func(tx *types.TransactionOrder, attr *TransferAttributes, ctx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateTransferTx(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid transfer tx: %w", err)
		}
		// calculate actual tx fee cost
		fee := m.feeCalculator()
		// update state
		updateDataFunc := updateBillDataFunc(tx, ctx.CurrentBlockNr, m.hashAlgorithm)
		setOwnerFunc := state.SetOwner(tx.UnitID(), attr.NewBearer)
		if err := m.state.Apply(
			setOwnerFunc,
			updateDataFunc,
		); err != nil {
			return nil, fmt.Errorf("transfer: failed to update state: %w", err)
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *Module) validateTransferTx(tx *types.TransactionOrder, attr *TransferAttributes) error {
	unit, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	if err := m.execPredicate(unit.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	return validateAnyTransfer(unit.Data(), attr.Backlink, attr.TargetValue)
}

func validateTransfer(data state.UnitData, attr *TransferAttributes) error {
	return validateAnyTransfer(data, attr.Backlink, attr.TargetValue)
}

func validateAnyTransfer(data state.UnitData, backlink []byte, targetValue uint64) error {
	bd, ok := data.(*BillData)
	if !ok {
		return ErrInvalidDataType
	}
	if bd.IsLocked() {
		return ErrBillLocked
	}
	if !bytes.Equal(backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	if targetValue != bd.V {
		return ErrInvalidBillValue
	}
	return nil
}

func updateBillDataFunc(tx *types.TransactionOrder, currentBlockNumber uint64, hashAlgorithm crypto.Hash) state.Action {
	unitID := tx.UnitID()
	return state.UpdateUnitData(unitID,
		func(data state.UnitData) (state.UnitData, error) {
			bd, ok := data.(*BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			bd.T = currentBlockNumber
			bd.Backlink = tx.Hash(hashAlgorithm)
			return bd, nil
		})
}
