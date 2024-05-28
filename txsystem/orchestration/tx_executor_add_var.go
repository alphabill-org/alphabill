package orchestration

import (
	"errors"
	"fmt"
	"math"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

// orchestration is defined currently with no fee handling, every tx cost is 0
type OrchestrationCtx struct {
	predicates.TxContext
}

func (o *OrchestrationCtx) GetGasRemaining() uint64   { return math.MaxUint64 }
func (o *OrchestrationCtx) SpendGas(gas uint64) error { return nil }

func (m *Module) executeAddVarTx(tx *types.TransactionOrder, attr *orchestration.AddVarAttributes, _ txsystem.ExecutionContext) (*types.ServerMetadata, error) {
	// try to update unit
	err := m.state.Apply(state.UpdateUnitData(tx.UnitID(),
		func(data types.UnitData) (types.UnitData, error) {
			vd, ok := data.(*orchestration.VarData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain var data", tx.UnitID())
			}
			vd.EpochNumber = attr.Var.EpochNumber
			return vd, nil
		}))
	// if unit is not created yet, update will return not found, in that case create the unit
	if err != nil && errors.Is(err, avl.ErrNotFound) {
		err = m.state.Apply(state.AddUnit(tx.UnitID(), m.ownerPredicate, &orchestration.VarData{EpochNumber: 0}))
	}
	// either update or add failed, report error and return
	if err != nil {
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

func (m *Module) validateAddVarTx(tx *types.TransactionOrder, attr *orchestration.AddVarAttributes, exeCtx txsystem.ExecutionContext) error {
	if !tx.UnitID().HasType(orchestration.VarUnitType) {
		return errors.New("invalid unit identifier: type is not VAR type")
	}
	unit, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	// if the unit does not exist yet then epoch must be 0
	if unit == nil {
		if attr.Var.EpochNumber != 0 {
			return fmt.Errorf("invalid epoch number, must be 0 for new units, got %d", attr.Var.EpochNumber)
		}
	} else {
		varData, ok := unit.Data().(*orchestration.VarData)
		if !ok {
			return errors.New("invalid unit data type")
		}
		if varData.EpochNumber != attr.Var.EpochNumber-1 {
			return fmt.Errorf("invalid epoch number, must increment by 1, got %d expected %d", attr.Var.EpochNumber, varData.EpochNumber+1)
		}
	}
	// Always check owner predicate, do it as a last step because it is the most expensive check
	orchCtx := &OrchestrationCtx{exeCtx}
	if err = m.execPredicate(m.ownerPredicate, tx.OwnerProof, tx, orchCtx); err != nil {
		return fmt.Errorf("invalid owner proof: %w", err)
	}
	return nil
}
