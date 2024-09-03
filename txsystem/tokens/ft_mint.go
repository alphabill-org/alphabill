package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *FungibleTokensModule) executeMintFT(tx *types.TransactionOrder, attr *tokens.MintFungibleTokenAttributes, _ *tokens.MintFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	tokenID := tx.UnitID()
	typeID := attr.TypeID

	if err := m.state.Apply(
		state.AddUnit(tokenID, attr.OwnerPredicate, tokens.NewFungibleTokenData(typeID, attr.Value, exeCtx.CurrentRound(), 0, tx.Timeout())),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{tokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateMintFT(tx *types.TransactionOrder, attr *tokens.MintFungibleTokenAttributes, authProof *tokens.MintFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	tokenID := tx.UnitID()
	tokenTypeID := attr.TypeID

	// verify tx.unitID (new token id) has correct embedded type
	if !tokenID.HasType(tokens.FungibleTokenUnitType) {
		return errors.New(ErrStrInvalidUnitID)
	}

	// verify token type has correct embedded type
	if !tokenTypeID.HasType(tokens.FungibleTokenTypeUnitType) {
		return errors.New(ErrStrInvalidTokenTypeID)
	}

	// verify token does not exist yet
	token, err := m.state.GetUnit(tokenID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("faild to load token: %w", err)
	}
	if token != nil {
		return fmt.Errorf("token already exists: %s", tokenID)
	}

	// verify token type does exist
	tokenType, err := m.state.GetUnit(tokenTypeID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if tokenType == nil {
		return fmt.Errorf("token type does not exist: %s", tokenTypeID)
	}
	tokenTypeData, ok := tokenType.Data().(*tokens.FungibleTokenTypeData)
	if !ok {
		return fmt.Errorf("token type data is not of type *tokens.FungibleTokenTypeData")
	}

	// verify new token has non-zero value
	if attr.Value == 0 {
		return errors.New("token must have value greater than zero")
	}

	// verify token id is correctly generated
	unitPart, err := tokens.HashForNewTokenID(tx, m.hashAlgorithm)
	if err != nil {
		return err
	}
	newTokenID := tokens.NewFungibleTokenID(tokenTypeID, unitPart)
	if !newTokenID.Eq(tokenID) {
		return errors.New("invalid token id")
	}

	// verify token minting predicate of the type
	if err := m.execPredicate(tokenTypeData.TokenMintingPredicate, authProof.TokenMintingProof, tx, exeCtx); err != nil {
		return fmt.Errorf(`executing FT type's "TokenMintingPredicate": %w`, err)
	}
	return nil
}
