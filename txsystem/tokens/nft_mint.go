package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (n *NonFungibleTokensModule) handleMintNonFungibleTokenTx() txsystem.GenericExecuteFunc[tokens.MintNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := n.validateMintNonFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid mint non-fungible token tx: %w", err)
		}
		fee := n.feeCalculator()
		typeID := tx.UnitID()
		newTokenID := tokens.NewNonFungibleTokenID(typeID, HashForIDCalculation(tx, n.hashAlgorithm))

		if err := n.state.Apply(
			state.AddUnit(newTokenID, attr.Bearer, tokens.NewNonFungibleTokenData(typeID, attr, exeCtx.CurrentBlockNr, 0)),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (n *NonFungibleTokensModule) validateMintNonFungibleToken(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	if len(attr.Name) > maxNameLength {
		return errors.New(ErrStrInvalidNameLength)
	}
	uri := attr.URI
	if uri != "" {
		if len(uri) > uriMaxSize {
			return fmt.Errorf("URI exceeds the maximum allowed size of %v KB", uriMaxSize)
		}
		if !util.IsValidURI(uri) {
			return fmt.Errorf("URI %s is invalid", uri)
		}
	}
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	_, err := n.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}

	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		tx,
		unitID,
		attr.TokenCreationPredicateSignatures,
		n.execPredicate,
		func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.TokenCreationPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf(`executing NFT type's "TokenCreationPredicate": %w`, err)
	}
	return nil
}
