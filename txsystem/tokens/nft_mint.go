package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (n *NonFungibleTokensModule) executeNFTMintTx(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, exeCtx txsystem.ExecutionContext) (*types.ServerMetadata, error) {
	fee := n.feeCalculator()

	if err := n.state.Apply(
		state.AddUnit(tx.UnitID(), attr.Bearer, tokens.NewNonFungibleTokenData(attr.TypeID, attr, exeCtx.CurrentRound(), 0)),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (n *NonFungibleTokensModule) validateNFTMintTx(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, exeCtx txsystem.ExecutionContext) error {
	tokenID := tx.UnitID()
	tokenTypeID := attr.TypeID

	if !tokenID.HasType(tokens.NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}

	// verify token type has correct embedded type
	if !tokenTypeID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidTokenTypeID)
	}

	// verify max allowed sizes
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

	// verify token does not exist yet
	token, err := n.state.GetUnit(tokenID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if token != nil {
		return fmt.Errorf("token already exists: %s", tokenID)
	}

	// verify token type does exist
	tokenType, err := n.state.GetUnit(tokenTypeID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if tokenType == nil {
		return fmt.Errorf("nft type does not exist: %s", tokenTypeID)
	}

	// verify token id is correctly generated
	unitPart, err := tokens.HashForNewTokenID(attr, tx.Payload.ClientMetadata, n.hashAlgorithm)
	if err != nil {
		return err
	}
	newTokenID := tokens.NewNonFungibleTokenID(tokenTypeID, unitPart)
	if !newTokenID.Eq(tokenID) {
		return errors.New("invalid token id")
	}

	// verify predicate inheritance chain
	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		exeCtx,
		tx,
		tokenTypeID,
		attr.TokenCreationPredicateSignatures,
		n.execPredicate,
		func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.TokenCreationPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf(`executing NFT type's "TokenCreationPredicate": %w`, err)
	}
	return nil
}
