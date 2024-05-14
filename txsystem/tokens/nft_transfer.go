package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (n *NonFungibleTokensModule) executeNFTTransferTx(tx *types.TransactionOrder, attr *tokens.TransferNonFungibleTokenAttributes, exeCtx txsystem.ExecutionContext) (*types.ServerMetadata, error) {
	fee := n.feeCalculator()
	unitID := tx.UnitID()
	// update owner, counter and last updated block number
	if err := n.state.Apply(
		state.SetOwner(unitID, attr.NewBearer),
		state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
			d, ok := data.(*tokens.NonFungibleTokenData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
			}
			d.T = exeCtx.CurrentRound()
			d.Counter += 1
			return d, nil
		}),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (n *NonFungibleTokensModule) validateNFTTransferTx(tx *types.TransactionOrder, attr *tokens.TransferNonFungibleTokenAttributes, exeCtx txsystem.ExecutionContext) error {
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
		return fmt.Errorf("validate nft transfer: unit %v is not a non-fungible token type", unitID)
	}
	if data.Locked != 0 {
		return errors.New("token is locked")
	}
	if data.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", data.Counter, attr.Counter)
	}
	tokenTypeID := data.NftType
	if !bytes.Equal(attr.NFTTypeID, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenTypeID, attr.NFTTypeID)
	}

	if err = n.execPredicate(u.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		exeCtx,
		tx,
		data.NftType,
		attr.InvariantPredicateSignatures,
		n.execPredicate,
		func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.InvariantPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf(`token type "InvariantPredicate": %w`, err)
	}
	return nil
}
