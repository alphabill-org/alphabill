package permissioned

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeCreditModule) validateSetFC(tx *types.TransactionOrder, attr *permissioned.SetFeeCreditAttributes, authProof *permissioned.SetFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	// verify there's no fee credit reference or separate fee authorization proof
	if err := feeModule.ValidateGenericFeeCreditTx(tx); err != nil {
		return err
	}

	// verify unit id has the correct type byte
	unitID := tx.GetUnitID()
	if err := unitID.TypeMustBe(f.feeCreditRecordUnitType, &f.pdr); err != nil {
		return fmt.Errorf("invalid unit type for unitID: %s", unitID)
	}

	fcrUnit, err := exeCtx.GetUnit(unitID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("failed to get unit: %w", err)
	}
	if fcrUnit != nil {
		fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
		if !ok {
			return fmt.Errorf("fee credit record unit data type is not of *fc.FeeCreditRecord type")
		}
		// verify owner predicate
		if !bytes.Equal(fcr.OwnerPredicate, attr.OwnerPredicate) {
			return fmt.Errorf("fee credit record owner predicate does not match the target owner predicate")
		}
		// verify counter
		if attr.Counter == nil {
			return errors.New("invalid counter: must not be nil when updating an existing fee credit record")
		}
		if fcr.GetCounter() != *attr.Counter {
			return fmt.Errorf("invalid counter: tx.Counter=%d fcr.Counter=%d", *attr.Counter, fcr.GetCounter())
		}
	} else {
		// verify unit id is correctly calculated
		fcrID, err := f.NewFeeCreditRecordID(unitID, attr.OwnerPredicate, tx.Timeout())
		if err != nil {
			return fmt.Errorf("failed to create fee credit record id: %w", err)
		}
		if !fcrID.Eq(unitID) {
			return fmt.Errorf("tx.unitID is not equal to expected fee credit record id (hash of fee credit owner predicate and tx.timeout), txUnitID=%s expectedUnitID=%s", unitID, fcrID)
		}
		// verify counter
		if attr.Counter != nil {
			return errors.New("invalid counter: must be nil when creating a new fee credit record")
		}
	}

	// verify tx is signed by admin key
	if err := f.execPredicate(f.adminOwnerPredicate, authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return fmt.Errorf("invalid owner proof: %w", err)
	}
	return nil
}

func (f *FeeCreditModule) executeSetFC(tx *types.TransactionOrder, attr *permissioned.SetFeeCreditAttributes, _ *permissioned.SetFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	fcrUnit, err := exeCtx.GetUnit(tx.UnitID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return nil, fmt.Errorf("failed to get unit: %w", err)
	}
	var actionFn state.Action
	if fcrUnit != nil {
		actionFn = unit.IncrCredit(tx.UnitID, attr.Amount, tx.Timeout())
	} else {
		fcr := fc.NewFeeCreditRecord(attr.Amount, attr.OwnerPredicate, tx.Timeout())
		actionFn = unit.AddCredit(tx.UnitID, fcr)
	}
	if err := f.state.Apply(actionFn); err != nil {
		return nil, fmt.Errorf("failed to set fee credit record: %w", err)
	}
	return &types.ServerMetadata{
		TargetUnits:      []types.UnitID{tx.UnitID},
		SuccessIndicator: types.TxStatusSuccessful,
	}, nil
}

func (f *FeeCreditModule) NewFeeCreditRecordID(unitID []byte, ownerPredicate []byte, timeout uint64) (types.UnitID, error) {
	return f.pdr.ComposeUnitID(types.ShardID{}, f.feeCreditRecordUnitType, fc.PrndSh(ownerPredicate, timeout))
}
