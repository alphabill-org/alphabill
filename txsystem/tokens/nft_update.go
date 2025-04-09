package tokens

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (n *NonFungibleTokensModule) executeUpdateNFT(tx *types.TransactionOrder, attr *tokens.UpdateNonFungibleTokenAttributes, _ *tokens.UpdateNonFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	if err := n.state.Apply(
		state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
			d, ok := data.(*tokens.NonFungibleTokenData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
			}
			d.Data = attr.Data
			d.Counter += 1
			return d, nil
		}),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (n *NonFungibleTokensModule) validateUpdateNFT(tx *types.TransactionOrder, attr *tokens.UpdateNonFungibleTokenAttributes, authProof *tokens.UpdateNonFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	unitID := tx.GetUnitID()
	if err := unitID.TypeMustBe(tokens.NonFungibleTokenUnitType, &n.pdr); err != nil {
		return fmt.Errorf("invalid unit ID: %w", err)
	}
	u, err := n.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*tokens.NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if data.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: got %d expected %d", attr.Counter, data.Counter)
	}

	exeCtx = exeCtx.WithExArg(tx.AuthProofSigBytes)
	if err = n.execPredicate(data.DataUpdatePredicate, authProof.TokenDataUpdateProof, tx, exeCtx); err != nil {
		return fmt.Errorf("data update predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		exeCtx,
		tx,
		data.TypeID,
		authProof.TokenTypeDataUpdateProofs,
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
