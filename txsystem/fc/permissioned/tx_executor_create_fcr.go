package permissioned

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeCreditModule) validateCreateFCR(tx *types.TransactionOrder, attr *permissioned.CreateFeeCreditAttributes, exeCtx txtypes.ExecutionContext) error {
	// verify tx.FeeProof is nil and tx.ClientMetadata.FeeCreditRecordID is nil
	if err := feeModule.ValidateGenericFeeCreditTx(tx); err != nil {
		return err
	}

	// verify unit id has the correct type byte
	unitID := tx.UnitID()
	if ok := unitID.HasType(f.feeCreditRecordUnitType); !ok {
		return fmt.Errorf("invalid unit type for unitID: %s", unitID)
	}

	// verify fee credit record is calculated correctly
	fcrID := f.NewFeeCreditRecordID(unitID, attr.FeeCreditOwnerCondition, tx.Timeout())
	if !fcrID.Eq(unitID) {
		return fmt.Errorf("tx.unitID is not equal to expected fee credit record id (hash of fee credit owner condition and tx.timeout), tx.UnitID=%s expected.fcrID=%s", unitID, fcrID)
	}

	// verify unit does not exist
	bd, err := exeCtx.GetUnit(unitID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("get fcr unit error: %w", err)
	}
	if bd != nil {
		return fmt.Errorf("fee credit record already exists: %s", unitID)
	}

	// verify tx is signed by admin key
	if err := f.execPredicate(f.adminOwnerCondition, tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("invalid owner proof: %w", err)
	}
	return nil
}

func (f *FeeCreditModule) executeCreateFCR(tx *types.TransactionOrder, attr *permissioned.CreateFeeCreditAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// create the fee credit record
	fcr := &fc.FeeCreditRecord{
		Balance: 1e8, // all "permissioned" fee credit records have hardcoded random chosen value of 1 alpha
		Timeout: tx.Timeout(),
	}
	if err := f.state.Apply(state.AddUnit(tx.UnitID(), attr.FeeCreditOwnerCondition, fcr)); err != nil {
		return nil, fmt.Errorf("failed to create fee credit record: %w", err)
	}
	return &types.ServerMetadata{
		TargetUnits:      []types.UnitID{tx.UnitID()},
		SuccessIndicator: types.TxStatusSuccessful,
	}, nil
}

func (f *FeeCreditModule) NewFeeCreditRecordID(unitID []byte, ownerPredicate []byte, timeout uint64) types.UnitID {
	unitPart := fc.NewFeeCreditRecordUnitPart(ownerPredicate, timeout)
	unitIdLen := len(unitPart) + len(f.feeCreditRecordUnitType)
	return types.NewUnitID(unitIdLen, unitID, unitPart, f.feeCreditRecordUnitType)
}
