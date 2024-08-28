package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *FungibleTokensModule) executeSplitFT(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, _ *tokens.SplitFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	u, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return nil, err
	}
	ftData := u.Data().(*tokens.FungibleTokenData)

	// add new token unit
	newTokenID := tokens.NewFungibleTokenID(unitID, tx.HashForNewUnitID(m.hashAlgorithm))

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

func HashForIDCalculation(tx *types.TransactionOrder, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	hasher.Write(tx.UnitID())
	hasher.Write(tx.Payload.Attributes)
	tx.Payload.ClientMetadata.AddToHasher(hasher)
	return hasher.Sum(nil)
}

func (m *FungibleTokensModule) validateSplitFT(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, authProof *tokens.SplitFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	ownerPredicate, tokenData, err := getFungibleTokenData(tx.UnitID(), m.state)
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

	payloadBytes, err := tx.PayloadBytes()
	if err != nil {
		return fmt.Errorf("failed to marshal payload bytes: %w", err)
	}
	if err = m.execPredicate(ownerPredicate, authProof.OwnerProof, payloadBytes, exeCtx); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		payloadBytes,
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
