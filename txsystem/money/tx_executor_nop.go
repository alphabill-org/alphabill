package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *Module) validateNopTx(tx *types.TransactionOrder, attr *nop.Attributes, _ *nop.AuthProof, _ txtypes.ExecutionContext) error {
	unitID := tx.GetUnitID()
	unit, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("nop transaction: get unit error: %w", err)
	}
	if err := m.verifyCounter(unit.Data(), attr); err != nil {
		return fmt.Errorf("nop transaction: %w", err)
	}
	return nil
}

func (m *Module) executeNopTx(tx *types.TransactionOrder, _ *nop.Attributes, _ *nop.AuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	action := state.UpdateUnitData(unitID, m.incrementCounterFn())
	if err := m.state.Apply(action); err != nil {
		return nil, fmt.Errorf("nop transaction: failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) verifyCounter(unitData types.UnitData, attr *nop.Attributes) error {
	if unitData == nil {
		if attr.Counter != nil {
			return ErrInvalidCounter
		}
		return nil
	}
	if attr.Counter == nil {
		return ErrInvalidCounter
	}
	var counter uint64
	switch data := unitData.(type) {
	case *fc.FeeCreditRecord:
		counter = data.Counter
	case *money.BillData:
		counter = data.Counter
	default:
		return errors.New("invalid unit data type")
	}
	if *attr.Counter != counter {
		return ErrInvalidCounter
	}
	return nil
}

func (m *Module) incrementCounterFn() func(data types.UnitData) (types.UnitData, error) {
	return func(data types.UnitData) (types.UnitData, error) {
		if data == nil {
			return nil, nil // do nothing if dummy unit
		}
		switch unitData := data.(type) {
		case *money.BillData:
			unitData.Counter += 1
			return unitData, nil
		case *fc.FeeCreditRecord:
			unitData.Counter += 1
			return unitData, nil
		default:
			return nil, fmt.Errorf("nop transaction: invalid unit data type")
		}
	}
}
