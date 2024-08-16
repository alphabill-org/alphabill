package permissioned

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeCreditModule) validateDeleteFCR(tx *types.TransactionOrder, _ *permissioned.DeleteFeeCreditAttributes, authProof *permissioned.DeleteFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	// verify tx.FeeProof is nil and tx.ClientMetadata.FeeCreditRecordID is nil
	if err := feeModule.ValidateGenericFeeCreditTx(tx); err != nil {
		return err
	}

	// verify unit id has the correct type byte
	unitID := tx.UnitID()
	if ok := unitID.HasType(f.feeCreditRecordUnitType); !ok {
		return fmt.Errorf("invalid unit type for unitID: %s", unitID)
	}

	// verify unit does exist (if unit does not exist then error is returned)
	_, err := exeCtx.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("get fcr unit error: %w", err)
	}

	// verify tx is signed by admin key
	payloadBytes, err := tx.PayloadBytes()
	if err != nil {
		return fmt.Errorf("failed to marshal payload bytes: %w", err)
	}
	if err := f.execPredicate(f.adminOwnerCondition, authProof.OwnerProof, payloadBytes, exeCtx); err != nil {
		return fmt.Errorf("invalid owner proof: %w", err)
	}
	return nil
}

func (f *FeeCreditModule) executeDeleteFCR(tx *types.TransactionOrder, _ *permissioned.DeleteFeeCreditAttributes, _ *permissioned.DeleteFeeCreditAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	setOwnerFn := state.SetOwner(tx.UnitID(), templates.AlwaysFalseBytes())
	if err := f.state.Apply(setOwnerFn); err != nil {
		return nil, fmt.Errorf("failed to delete fee credit record: %w", err)
	}
	return &types.ServerMetadata{
		TargetUnits:      []types.UnitID{tx.UnitID()},
		SuccessIndicator: types.TxStatusSuccessful,
	}, nil
}
