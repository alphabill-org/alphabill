package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (n *NonFungibleTokensModule) executeMintNFT(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, _ *tokens.MintNonFungibleTokenAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	if err := n.state.Apply(
		state.AddUnit(tx.UnitID, tokens.NewNonFungibleTokenData(attr.TypeID, attr)),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{tx.UnitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (n *NonFungibleTokensModule) validateMintNFT(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, authProof *tokens.MintNonFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	tokenID := tx.GetUnitID()
	tokenTypeID := attr.TypeID

	if !tokenID.HasType(tokens.NonFungibleTokenUnitType) {
		return errors.New(ErrStrInvalidUnitID)
	}

	// verify token type has correct embedded type
	if !tokenTypeID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return errors.New(ErrStrInvalidTokenTypeID)
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
	tokenTypeData, ok := tokenType.Data().(*tokens.NonFungibleTokenTypeData)
	if !ok {
		return fmt.Errorf("token type data is not of type *tokens.NonFungibleTokenTypeData")
	}

	// verify token id is correctly generated
	unitPart, err := tokens.HashForNewTokenID(tx, n.hashAlgorithm)
	if err != nil {
		return err
	}
	newTokenID := tokens.NewNonFungibleTokenID(tokenTypeID, unitPart)
	if !newTokenID.Eq(tokenID) {
		return errors.New("invalid token id")
	}

	// verify token minting predicate of the type
	if err := n.execPredicate(tokenTypeData.TokenMintingPredicate, authProof.TokenMintingProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf(`executing NFT type's "TokenMintingPredicate": %w`, err)
	}
	return nil
}
