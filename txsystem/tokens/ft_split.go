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
	newTokenID, err := m.pdr.ComposeUnitID(types.ShardID{}, tokens.FungibleTokenUnitType, tokens.PrndSh(tx))
	if err != nil {
		return nil, err
	}

	// update state
	if err = m.state.Apply(
		state.AddUnit(newTokenID, &tokens.FungibleTokenData{
			TypeID:         ftData.TypeID,
			Value:          attr.TargetValue,
			OwnerPredicate: attr.NewOwnerPredicate,
		}),
		state.UpdateUnitData(unitID,
			func(data types.UnitData) (types.UnitData, error) {
				d, ok := data.(*tokens.FungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
				}
				// 3. N[T.ι].D.v ← N[T.ι].D.v − T.A.v
				// 4. N[T.ι].D.c ← N[T.ι].D.c + 1
				d.Value -= attr.TargetValue
				d.Counter += 1
				return d, nil
			}),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID, newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateSplitFT(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, authProof *tokens.SplitFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	tokenData, err := m.getFungibleTokenData(tx.UnitID)
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
	if !bytes.Equal(attr.TypeID, tokenData.TypeID) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenData.TypeID, attr.TypeID)
	}

	exeCtx = exeCtx.WithExArg(tx.AuthProofSigBytes)
	if err = m.execPredicate(tokenData.OwnerPredicate, authProof.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		tx,
		tokenData.TypeID,
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
