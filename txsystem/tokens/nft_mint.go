package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (n *NonFungibleTokensModule) executeNFTMintTx(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
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

func (n *NonFungibleTokensModule) validateNFTMintTx(tx *types.TransactionOrder, attr *tokens.MintNonFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) error {
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
		exeCtx,
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
