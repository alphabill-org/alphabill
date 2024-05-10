package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *FungibleTokensModule) executeMintFT(tx *types.TransactionOrder, attr *tokens.MintFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
	fee := m.feeCalculator()
	typeID := tx.UnitID()
	newTokenID := tokens.NewFungibleTokenID(typeID, HashForIDCalculation(tx, m.hashAlgorithm))

	if err := m.state.Apply(
		state.AddUnit(newTokenID, attr.Bearer, tokens.NewFungibleTokenData(typeID, attr.Value, exeCtx.CurrentBlockNr, 0, tx.Timeout())),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateMintFT(tx *types.TransactionOrder, attr *tokens.MintFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) error {
	unitID := tx.UnitID()
	if !unitID.HasType(tokens.FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	_, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	if attr.Value == 0 {
		return errors.New("token must have value greater than zero")
	}
	if err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		tx,
		tx.UnitID(),
		attr.TokenCreationPredicateSignatures,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.TokenCreationPredicate
		},
		m.state.GetUnit,
	); err != nil {
		return fmt.Errorf("evaluating TokenCreationPredicate: %w", err)
	}
	return nil
}
