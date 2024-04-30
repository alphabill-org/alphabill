package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *FungibleTokensModule) handleBurnFungibleTokenTx() txsystem.GenericExecuteFunc[tokens.BurnFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.BurnFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateBurnFungibleToken(tx, attr, exeCtx); err != nil {
			return nil, fmt.Errorf("invalid burn fungible token transaction: %w", err)
		}
		fee := m.feeCalculator()
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
				ftData.T = exeCtx.CurrentBlockNr
				ftData.Counter += 1
				return ftData, nil
			},
		)

		if err := m.state.Apply(setOwnerFn, updateUnitFn); err != nil {
			return nil, fmt.Errorf("burnFToken: failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateBurnFungibleToken(tx *types.TransactionOrder, attr *tokens.BurnFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return err
	}
	if d.Locked != 0 {
		return errors.New("token is locked")
	}
	if !bytes.Equal(d.TokenType, attr.TypeID) {
		return fmt.Errorf("type of token to burn does not matches the actual type of the token: expected %s, got %s", d.TokenType, attr.TypeID)
	}
	if attr.Value != d.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.Value, attr.Value)
	}
	if d.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", d.Counter, attr.Counter)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("bearer predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		tx,
		d.TokenType,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return nil
}
