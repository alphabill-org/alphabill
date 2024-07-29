package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

func (m *FungibleTokensModule) executeBurnFT(tx *types.TransactionOrder, _ *tokens.BurnFungibleTokenAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()

	// 1. SetOwner(ι, DC)
	setOwnerFn := state.SetOwner(unitID, templates.AlwaysFalseBytes())

	// 2. UpdateData(ι, f), where f(D) = (0, S.n, H(P))
	updateUnitFn := state.UpdateUnitData(unitID,
		func(data types.UnitData) (types.UnitData, error) {
			ftData, ok := data.(*tokens.FungibleTokenData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
			}
			ftData.Value = 0
			ftData.T = exeCtx.CurrentRound()
			ftData.Counter += 1
			return ftData, nil
		},
	)
	if err := m.state.Apply(setOwnerFn, updateUnitFn); err != nil {
		return nil, fmt.Errorf("burnFToken: failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateBurnFT(tx *types.TransactionOrder, attr *tokens.BurnFungibleTokenAttributes, exeCtx txtypes.ExecutionContext) error {
	bearer, tokenData, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return err
	}
	if tokenData.Locked != 0 {
		return errors.New("token is locked")
	}
	if !bytes.Equal(tokenData.TokenType, attr.TypeID) {
		return fmt.Errorf("type of token to burn does not matches the actual type of the token: expected %s, got %s", tokenData.TokenType, attr.TypeID)
	}
	if attr.Value != tokenData.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", tokenData.Value, attr.Value)
	}
	if tokenData.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", tokenData.Counter, attr.Counter)
	}

	err = m.execPredicate(bearer, tx.OwnerProof, tx, exeCtx)
	if err != nil {
		return fmt.Errorf("bearer predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		tx,
		tokenData.TokenType,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return nil
}
