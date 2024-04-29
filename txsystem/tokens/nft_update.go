package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (n *NonFungibleTokensModule) handleUpdateNonFungibleTokenTx() txsystem.GenericExecuteFunc[UpdateNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (sm *types.ServerMetadata, err error) {
		isLocked := false
		if !exeCtx.StateLockReleased {
			if err = n.validateUpdateNonFungibleToken(tx, attr, exeCtx); err != nil {
				return nil, fmt.Errorf("invalid update non-fungible token tx: %w", err)
			}
			isLocked, err = txsystem.LockUnitState(tx, n.execPredicate, n.state, exeCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to lock unit state: %w", err)
			}
		}
		fee := n.feeCalculator()
		unitID := tx.UnitID()

		// update state
		if !isLocked {
			if err = n.state.Apply(
				state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
					d, ok := data.(*NonFungibleTokenData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
					}
					d.Data = attr.Data
					return d, nil
				})); err != nil {
				return nil, err
			}
		}

		if err = n.state.Apply(
			state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
				d, ok := data.(*NonFungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
				}
				d.T = exeCtx.CurrentBlockNr
				d.Counter += 1
				return d, nil
			})); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (n *NonFungibleTokensModule) validateUpdateNonFungibleToken(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) error {
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	unitID := tx.UnitID()
	if !unitID.HasType(NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := n.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if data.Locked != 0 {
		return errors.New("token is locked")
	}
	if data.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: got %d expected %d", attr.Counter, data.Counter)
	}

	if len(attr.DataUpdateSignatures) == 0 {
		return errors.New("missing data update signatures")
	}

	if err = n.execPredicate(data.DataUpdatePredicate, attr.DataUpdateSignatures[0], tx, exeCtx); err != nil {
		return fmt.Errorf("data update predicate: %w", err)
	}
	err = runChainedPredicates[*NonFungibleTokenTypeData](
		exeCtx,
		tx,
		data.NftType,
		attr.DataUpdateSignatures[1:],
		n.execPredicate,
		func(d *NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.DataUpdatePredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf(`token type DataUpdatePredicate: %w`, err)
	}
	return nil
}

func (u *UpdateNonFungibleTokenAttributes) GetData() []byte {
	return u.Data
}

func (u *UpdateNonFungibleTokenAttributes) SetData(data []byte) {
	u.Data = data
}

func (u *UpdateNonFungibleTokenAttributes) GetCounter() uint64 {
	return u.Counter
}

func (u *UpdateNonFungibleTokenAttributes) SetCounter(counter uint64) {
	u.Counter = counter
}

func (u *UpdateNonFungibleTokenAttributes) GetDataUpdateSignatures() [][]byte {
	return u.DataUpdateSignatures
}

func (u *UpdateNonFungibleTokenAttributes) SetDataUpdateSignatures(signatures [][]byte) {
	u.DataUpdateSignatures = signatures
}

func (u *UpdateNonFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude DataUpdateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &UpdateNonFungibleTokenAttributes{
		Data:                 u.Data,
		Counter:              u.Counter,
		DataUpdateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}
