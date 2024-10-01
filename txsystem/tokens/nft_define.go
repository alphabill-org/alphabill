package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (n *NonFungibleTokensModule) executeDefineNFT(tx *types.TransactionOrder, attr *tokens.DefineNonFungibleTokenAttributes, _ *tokens.DefineNonFungibleTokenAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// update state
	unitID := tx.GetUnitID()
	if err := n.state.Apply(
		state.AddUnit(unitID, templates.AlwaysTrueBytes(), tokens.NewNonFungibleTokenTypeData(attr)),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (n *NonFungibleTokensModule) validateDefineNFT(tx *types.TransactionOrder, attr *tokens.DefineNonFungibleTokenAttributes, authProof *tokens.DefineNonFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	unitID := tx.GetUnitID()
	if !unitID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidUnitID)
	}
	if attr.ParentTypeID != nil && !attr.ParentTypeID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidParentTypeID)
	}
	if len(attr.Symbol) > maxSymbolLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidSymbolLength)
	}
	if len(attr.Name) > maxNameLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidNameLength)
	}
	if attr.Icon != nil {
		if len(attr.Icon.Type) > maxIconTypeLength {
			return fmt.Errorf("create nft type: %s", ErrStrInvalidIconTypeLength)
		}
		if len(attr.Icon.Data) > maxIconDataLength {
			return fmt.Errorf("create nft type: %s", ErrStrInvalidIconDataLength)
		}
	}
	u, err := n.state.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("create nft type: unit %v exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		exeCtx,
		tx.AuthProofSigBytes,
		attr.ParentTypeID,
		authProof.SubTypeCreationProofs,
		n.execPredicate,
		func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.SubTypeCreationPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type SubTypeCreationPredicate: %w", err)
	}
	return nil
}
