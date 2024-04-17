package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (n *NonFungibleTokensModule) handleTransferNonFungibleTokenTx() txsystem.GenericExecuteFunc[tokens.TransferNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.TransferNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := n.validateTransferNonFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid transfer non-fungible token tx: %w", err)
		}
		fee := n.feeCalculator()

		unitID := tx.UnitID()

		// update state
		if err := n.state.Apply(
			state.SetOwner(unitID, attr.NewBearer),
			state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
				d, ok := data.(*tokens.NonFungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
				}
				d.T = currentBlockNr
				d.Backlink = tx.Hash(n.hashAlgorithm)
				return d, nil
			})); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (n *NonFungibleTokensModule) validateTransferNonFungibleToken(tx *types.TransactionOrder, attr *tokens.TransferNonFungibleTokenAttributes) error {
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
	if !bytes.Equal(data.Backlink, attr.Backlink) {
		return errors.New("validate nft transfer: invalid backlink")
	}
	tokenTypeID := data.NftType
	if !bytes.Equal(attr.NFTTypeID, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenTypeID, attr.NFTTypeID)
	}

	if err = n.execPredicate(u.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
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
