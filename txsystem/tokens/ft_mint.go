package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *FungibleTokensModule) handleMintFungibleTokenTx() txsystem.GenericExecuteFunc[tokens.MintFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.MintFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := m.validateMintFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid mint fungible token tx: %w", err)
		}
		fee := m.feeCalculator()

		unitID := tx.UnitID()
		h := tx.Hash(m.hashAlgorithm)

		// update state
		if err := m.state.Apply(
			state.AddUnit(unitID, attr.Bearer, tokens.NewFungibleTokenData(attr, h, currentBlockNr)),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateMintFungibleToken(tx *types.TransactionOrder, attr *tokens.MintFungibleTokenAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(tokens.FungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := m.state.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("unit with id %v already exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if attr.Value == 0 {
		return errors.New("token must have value greater than zero")
	}
	if !attr.TypeID.HasType(tokens.FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidTypeID)
	}

	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		tx,
		attr.TypeID,
		attr.TokenCreationPredicateSignatures,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.TokenCreationPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("evaluating TokenCreationPredicate: %w", err)
	}
	return nil
}
