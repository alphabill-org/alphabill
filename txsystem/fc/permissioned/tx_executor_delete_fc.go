package permissioned

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeCreditModule) validateDeleteFC(tx *types.TransactionOrder, attr *permissioned.DeleteFeeCreditAttributes, authProof *permissioned.DeleteFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	// verify there's no fee credit reference or separate fee authorization proof
	if err := feeModule.ValidateGenericFeeCreditTx(tx); err != nil {
		return err
	}

	// verify unit id has the correct type byte
	unitID := tx.GetUnitID()
	if ok := unitID.HasType(f.feeCreditRecordUnitType); !ok {
		return fmt.Errorf("invalid unit type for unitID: %s", unitID)
	}

	// verify fee credit record exists (if unit does not exist then error is returned)
	fcrUnit, err := exeCtx.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("failed to get unit: %w", err)
	}
	// just in case do a nil check as well
	if fcrUnit == nil {
		return fmt.Errorf("unit %s does not exist", unitID)
	}

	// verify counter
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	if !ok {
		return fmt.Errorf("fee credit record unit data type is not of *fc.FeeCreditRecord type")
	}
	if fcr.GetCounter() != attr.Counter {
		return fmt.Errorf("invalid counter: tx.Counter=%d fcr.Counter=%d", attr.Counter, fcr.GetCounter())
	}

	// verify tx is signed by admin key
	if err := f.execPredicate(f.adminOwnerPredicate, authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("invalid owner proof: %w", err)
	}
	return nil
}

func (f *FeeCreditModule) executeDeleteFC(tx *types.TransactionOrder, _ *permissioned.DeleteFeeCreditAttributes, _ *permissioned.DeleteFeeCreditAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	setOwnerFn := state.SetOwner(tx.UnitID, templates.AlwaysFalseBytes())
	if err := f.state.Apply(setOwnerFn); err != nil {
		return nil, fmt.Errorf("failed to delete fee credit record: %w", err)
	}
	return &types.ServerMetadata{
		TargetUnits:      []types.UnitID{tx.UnitID},
		SuccessIndicator: types.TxStatusSuccessful,
	}, nil
}
