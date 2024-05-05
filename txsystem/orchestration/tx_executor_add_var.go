package orchestration

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *Module) handleAddVarTx() txsystem.GenericExecuteFunc[orchestration.AddVarAttributes] {
	return func(tx *types.TransactionOrder, attr *orchestration.AddVarAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		unit, err := m.state.GetUnit(tx.UnitID(), false)
		if err != nil && !errors.Is(err, avl.ErrNotFound) {
			return nil, err
		}
		if err := m.validateAddVarTx(tx, attr, unit); err != nil {
			return nil, fmt.Errorf("invalid 'addVar' tx: %w", err)
		}
		var actions []state.Action

		// add validator assignment record if it does not already exist
		if unit == nil {
			addUnitFunc := state.AddUnit(tx.UnitID(), m.ownerPredicate, &orchestration.VarData{EpochNumber: 0})
			actions = append(actions, addUnitFunc)
		}

		// update validator assigment record epoch number
		updateUnitFunc := state.UpdateUnitData(tx.UnitID(),
			func(data types.UnitData) (types.UnitData, error) {
				vd, ok := data.(*orchestration.VarData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain var data", tx.UnitID())
				}
				vd.EpochNumber = attr.Var.EpochNumber
				return vd, nil
			})
		actions = append(actions, updateUnitFunc)

		if err := m.state.Apply(actions...); err != nil {
			return nil, fmt.Errorf("addVar: failed to update state: %w", err)
		}

		processingDetails, err := types.Cbor.Marshal(attr.Var)
		if err != nil {
			return nil, fmt.Errorf("addVar: failed to encode tx processing result: %w", err)
		}
		return &types.ServerMetadata{
			TargetUnits:       []types.UnitID{tx.UnitID()},
			SuccessIndicator:  types.TxStatusSuccessful,
			ProcessingDetails: processingDetails,
		}, nil
	}
}

func (m *Module) validateAddVarTx(tx *types.TransactionOrder, attr *orchestration.AddVarAttributes, unit *state.Unit) error {
	if !tx.UnitID().HasType(orchestration.VarUnitType) {
		return errors.New("invalid unit identifier: type is not VAR type")
	}
	if unit != nil {
		varData, ok := unit.Data().(*orchestration.VarData)
		if !ok {
			return errors.New("invalid unit data type")
		}
		if varData.EpochNumber != attr.Var.EpochNumber-1 {
			return fmt.Errorf("invalid epoch number, must increment by 1, got %d expected %d", attr.Var.EpochNumber, varData.EpochNumber+1)
		}
		return nil
	} else {
		if attr.Var.EpochNumber != 0 {
			return fmt.Errorf("invalid epoch number, must be 0 for new units, got %d", attr.Var.EpochNumber)
		}
		if err := m.execPredicate(m.ownerPredicate, tx.OwnerProof, tx); err != nil {
			return fmt.Errorf("invalid owner proof: %w", err)
		}
		return nil
	}
}
