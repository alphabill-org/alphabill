package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (n *NonFungibleTokensModule) executeNFTUpdateTx(tx *types.TransactionOrder, attr *tokens.UpdateNonFungibleTokenAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	// update state
	if err := n.state.Apply(
		state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
			d, ok := data.(*tokens.NonFungibleTokenData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
			}
			d.Data = attr.Data
			d.T = exeCtx.CurrentRound()
			d.Counter += 1
			return d, nil
		})); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (n *NonFungibleTokensModule) validateNFTUpdateTx(tx *types.TransactionOrder, attr *tokens.UpdateNonFungibleTokenAttributes, exeCtx txtypes.ExecutionContext) error {
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	unitID := tx.UnitID()
	if !unitID.HasType(tokens.NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := n.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*tokens.NonFungibleTokenData)
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
	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		exeCtx,
		tx,
		data.TypeID,
		attr.DataUpdateSignatures[1:],
		n.execPredicate,
		func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.DataUpdatePredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf(`token type DataUpdatePredicate: %w`, err)
	}
	return nil
}
