package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (n *NonFungibleTokensModule) handleMintNonFungibleTokenTx() txsystem.GenericExecuteFunc[tokens.MintNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := n.validateMintNonFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid mint non-fungible token tx: %w", err)
		}
		fee := n.feeCalculator()
		unitID := tx.UnitID()
		h := tx.Hash(n.hashAlgorithm)

		// update state
		if err := n.state.Apply(
			state.AddUnit(unitID, attr.Bearer, tokens.NewNonFungibleTokenData(attr, h, currentBlockNr))); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (n *NonFungibleTokensModule) validateMintNonFungibleToken(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(tokens.NonFungibleTokenUnitType) {
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
	u, err := n.state.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("unit %v exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if !attr.NFTTypeID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidTypeID)
	}

	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		tx,
		attr.NFTTypeID,
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
