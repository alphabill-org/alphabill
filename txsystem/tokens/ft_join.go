package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *FungibleTokensModule) executeJoinFT(tx *types.TransactionOrder, _ *tokens.JoinFungibleTokenAttributes, _ *tokens.JoinFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	sum := util.BytesToUint64(exeCtx.GetData())
	// update state
	if err := m.state.Apply(
		state.UpdateUnitData(unitID,
			func(data types.UnitData) (types.UnitData, error) {
				tokenData, ok := data.(*tokens.FungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
				}
				return &tokens.FungibleTokenData{
					TokenType: tokenData.TokenType,
					Value:     sum,
					T:         exeCtx.CurrentRound(),
					Counter:   tokenData.Counter + 1,
					Locked:    0,
				}, nil
			},
		),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateJoinFT(tx *types.TransactionOrder, attr *tokens.JoinFungibleTokenAttributes, authProof *tokens.JoinFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	ownerPredicate, tokenData, err := getFungibleTokenData(tx.UnitID, m.state)
	if err != nil {
		return err
	}
	sum := tokenData.Value
	for i, btx := range attr.BurnTokenProofs {
		btxAttr := &tokens.BurnFungibleTokenAttributes{}
		if err := btx.TransactionOrder().UnmarshalAttributes(btxAttr); err != nil {
			return fmt.Errorf("failed to unmarshal burn fungible token attributes")
		}

		var ok bool
		sum, ok = util.SafeAdd(sum, btxAttr.Value)
		if !ok {
			return errors.New("invalid sum of tokens: uint64 overflow")
		}

		if i > 0 && btx.UnitID().Compare(attr.BurnTokenProofs[i-1].UnitID()) != 1 {
			// burning transactions orders are listed in strictly increasing order of token identifiers
			// this ensures that no source token can be included multiple times
			return errors.New("burn transaction orders are not listed in strictly increasing order of token identifiers")
		}
		if !bytes.Equal(btxAttr.TypeID, tokenData.TokenType) {
			return fmt.Errorf("the type of the burned source token does not match the type of target token: expected %s, got %s", tokenData.TokenType, btxAttr.TypeID)
		}
		if !bytes.Equal(btxAttr.TargetTokenID, tx.UnitID) {
			return fmt.Errorf("burn transaction target token id does not match with join transaction unit id: burn transaction %X, join transaction %X", btxAttr.TargetTokenID, tx.UnitID)
		}
		if btxAttr.TargetTokenCounter != tokenData.Counter {
			return fmt.Errorf("burn transaction target token counter does not match with target unit counter: burn transaction counter %d, unit counter %d", btxAttr.TargetTokenCounter, tokenData.Counter)
		}
		if err = types.VerifyTxProof(btx, m.trustBase, m.hashAlgorithm); err != nil {
			return fmt.Errorf("proof is not valid: %w", err)
		}
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

	// add sum of token values to tx context
	exeCtx.SetData(util.Uint64ToBytes(sum))

	return nil
}
