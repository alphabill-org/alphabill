package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *FungibleTokensModule) executeSplitFT(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, _ *tokens.SplitFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return nil, err
	}
	ftData := u.Data().(*tokens.FungibleTokenData)

	// add new token unit
	unitPart, err := tokens.HashForNewTokenID(tx, m.hashAlgorithm)
	if err != nil {
		return nil, err
	}
	newTokenID := tokens.NewFungibleTokenID(unitID, unitPart)

	// update state
	if err = m.state.Apply(
		state.AddUnit(newTokenID,
			attr.NewOwnerPredicate,
			&tokens.FungibleTokenData{
				TokenType: ftData.TokenType,
				Value:     attr.TargetValue,
				T:         exeCtx.CurrentRound(),
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
					T:         exeCtx.CurrentRound(),
					Counter:   d.Counter + 1,
					T1:        d.T1,
					Locked:    d.Locked,
				}, nil
			}),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID, newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateSplitFT(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, authProof *tokens.SplitFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	ownerPredicate, tokenData, err := getFungibleTokenData(tx.UnitID, m.state)
	if err != nil {
		return err
	}
	if tokenData.Locked != 0 {
		return errors.New("token is locked")
	}
	if attr.TargetValue == 0 {
		return errors.New("when splitting a token the value assigned to the new token must be greater than zero")
	}
	if attr.TargetValue >= tokenData.Value {
		return fmt.Errorf("the target value must be less than the value of the source token: targetValue=%d tokenValue=%d", attr.TargetValue, tokenData.Value)
	}
	if tokenData.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", tokenData.Counter, attr.Counter)
	}
	if !bytes.Equal(attr.TypeID, tokenData.TokenType) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenData.TokenType, attr.TypeID)
	}

	if err = m.execPredicate(ownerPredicate, authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		tx.AuthProofSigBytes,
		tokenData.TokenType,
		authProof.TokenTypeOwnerProofs,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.TokenTypeOwnerPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("evaluating TokenTypeOwnerPredicate: %w", err)
	}
	return nil
}
