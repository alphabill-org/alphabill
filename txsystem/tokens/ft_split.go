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

func (m *FungibleTokensModule) executeSplitFT(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return nil, err
	}
	ftData := u.Data().(*tokens.FungibleTokenData)

	// add new token unit
	newTokenID := tokens.NewFungibleTokenID(unitID, tx.HashForNewUnitID(m.hashAlgorithm))
	fee := m.feeCalculator()

	// update state
	if err = m.state.Apply(
		state.AddUnit(newTokenID,
			attr.NewBearer,
			&tokens.FungibleTokenData{
				TokenType: ftData.TokenType,
				Value:     attr.TargetValue,
				T:         exeCtx.CurrentBlockNumber,
				Counter:   0,
				T1:        0,
				Locked:    0,
			}),
		state.UpdateUnitData(unitID,
			func(data types.UnitData) (types.UnitData, error) {
				d, ok := data.(*tokens.FungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
				}
				return &tokens.FungibleTokenData{
					TokenType: d.TokenType,
					Value:     d.Value - attr.TargetValue,
					T:         exeCtx.CurrentBlockNumber,
					Counter:   d.Counter + 1,
					T1:        d.T1,
					Locked:    d.Locked,
				}, nil
			}),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID, newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateSplitFT(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) error {
	bearer, tokenData, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return err
	}
	if tokenData.Locked != 0 {
		return errors.New("token is locked")
	}
	if attr.TargetValue == 0 {
		return errors.New("when splitting a token the value assigned to the new token must be greater than zero")
	}
	if attr.RemainingValue == 0 {
		return errors.New("when splitting a token the remaining value of the token must be greater than zero")
	}

	if tokenData.Value < attr.TargetValue {
		return fmt.Errorf("invalid token value: max allowed %d, got %d", tokenData.Value, attr.TargetValue)
	}
	remainingValue := tokenData.Value - attr.TargetValue
	if remainingValue != attr.RemainingValue {
		return errors.New("remaining value must equal to the original value minus target value")
	}

	if tokenData.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", tokenData.Counter, attr.Counter)
	}
	if !bytes.Equal(attr.TypeID, tokenData.TokenType) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenData.TokenType, attr.TypeID)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
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
		return fmt.Errorf("evaluating InvariantPredicate: %w", err)
	}
	return nil
}
