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

func (m *FungibleTokensModule) executeJoinFT(tx *types.TransactionOrder, attr *tokens.JoinFungibleTokenAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	// Todo: maybe instead of count the attributes should have sum value?
	// at this point sum must be computed twice as during conditional execution validation is not done

	sum := uint64(0)
	for _, btx := range attr.BurnTransactions {
		btxAttr := &tokens.BurnFungibleTokenAttributes{}
		if err := btx.TransactionOrder.UnmarshalAttributes(btxAttr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal burn fungible token attributes")
		}
		sum += btxAttr.Value
	}
	// update state
	if err := m.state.Apply(
		state.UpdateUnitData(unitID,
			func(data types.UnitData) (types.UnitData, error) {
				d, ok := data.(*tokens.FungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
				}
				return &tokens.FungibleTokenData{
					TokenType: d.TokenType,
					Value:     d.Value + sum,
					T:         exeCtx.CurrentRound(),
					Counter:   d.Counter + 1,
					Locked:    0,
				}, nil
			},
		),
	); err != nil {
		return nil, err
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateJoinFT(tx *types.TransactionOrder, attr *tokens.JoinFungibleTokenAttributes, exeCtx txtypes.ExecutionContext) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return err
	}
	transactions := attr.BurnTransactions
	proofs := attr.Proofs
	if len(transactions) != len(proofs) {
		return fmt.Errorf("invalid count of proofs: expected %v, got %v", len(transactions), len(proofs))
	}
	sum := d.Value
	for i, btx := range transactions {
		prevSum := sum
		btxAttr := &tokens.BurnFungibleTokenAttributes{}
		if err := btx.TransactionOrder.UnmarshalAttributes(btxAttr); err != nil {
			return fmt.Errorf("failed to unmarshal burn fungible token attributes")
		}

		sum += btxAttr.Value
		if prevSum > sum { // overflow
			return errors.New("invalid sum of tokens: uint64 overflow")
		}
		if i > 0 && btx.TransactionOrder.UnitID().Compare(transactions[i-1].TransactionOrder.UnitID()) != 1 {
			// burning transactions orders are listed in strictly increasing order of token identifiers
			// this ensures that no source token can be included multiple times
			return errors.New("burn tx orders are not listed in strictly increasing order of token identifiers")
		}
		if !bytes.Equal(btxAttr.TypeID, d.TokenType) {
			return fmt.Errorf("the type of the burned source token does not match the type of target token: expected %s, got %s", d.TokenType, btxAttr.TypeID)
		}
		if !bytes.Equal(btxAttr.TargetTokenID, tx.UnitID()) {
			return fmt.Errorf("burn tx target token id does not match with join transaction unit id: burnTx %X, joinTx %X", btxAttr.TargetTokenID, tx.UnitID())
		}
		if btxAttr.TargetTokenCounter != attr.Counter {
			return fmt.Errorf("burn tx target token counter does not match with join transaction counter: burnTx %d, joinTx %d", btxAttr.TargetTokenCounter, attr.Counter)
		}
		if err = types.VerifyTxProof(proofs[i], btx, m.trustBase, m.hashAlgorithm); err != nil {
			return fmt.Errorf("proof is not valid: %w", err)
		}
	}
	if d.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %X, got %X", d.Counter, attr.Counter)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		tx,
		d.TokenType,
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
