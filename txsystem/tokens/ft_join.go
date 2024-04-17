package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *FungibleTokensModule) handleJoinFungibleTokenTx() txsystem.GenericExecuteFunc[tokens.JoinFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.JoinFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		sum, err := m.validateJoinFungibleToken(tx, attr)
		if err != nil {
			return nil, fmt.Errorf("invalid join fungible token tx: %w", err)
		}
		fee := m.feeCalculator()

		// update state
		unitID := tx.UnitID()
		h := tx.Hash(m.hashAlgorithm)
		if err := m.state.Apply(
			state.UpdateUnitData(unitID,
				func(data types.UnitData) (types.UnitData, error) {
					d, ok := data.(*tokens.FungibleTokenData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
					}
					return &tokens.FungibleTokenData{
						TokenType: d.TokenType,
						Value:     sum,
						T:         currentBlockNr,
						Backlink:  h,
						Locked:    0,
					}, nil
				})); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateJoinFungibleToken(tx *types.TransactionOrder, attr *tokens.JoinFungibleTokenAttributes) (uint64, error) {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return 0, err
	}
	transactions := attr.BurnTransactions
	proofs := attr.Proofs
	if len(transactions) != len(proofs) {
		return 0, fmt.Errorf("invalid count of proofs: expected %v, got %v", len(transactions), len(proofs))
	}
	sum := d.Value
	for i, btx := range transactions {
		prevSum := sum
		btxAttr := &tokens.BurnFungibleTokenAttributes{}
		if err := btx.TransactionOrder.UnmarshalAttributes(btxAttr); err != nil {
			return 0, fmt.Errorf("failed to unmarshal burn fungible token attributes")
		}

		sum += btxAttr.Value
		if prevSum > sum { // overflow
			return 0, errors.New("invalid sum of tokens: uint64 overflow")
		}
		if i > 0 && btx.TransactionOrder.UnitID().Compare(transactions[i-1].TransactionOrder.UnitID()) != 1 {
			// burning transactions orders are listed in strictly increasing order of token identifiers
			// this ensures that no source token can be included multiple times
			return 0, errors.New("burn tx orders are not listed in strictly increasing order of token identifiers")
		}
		if !bytes.Equal(btxAttr.TypeID, d.TokenType) {
			return 0, fmt.Errorf("the type of the burned source token does not match the type of target token: expected %s, got %s", d.TokenType, btxAttr.TypeID)
		}
		if !bytes.Equal(btxAttr.TargetTokenID, tx.UnitID()) {
			return 0, fmt.Errorf("burn tx target token id does not match with join transaction unit id: burnTx %X, joinTx %X", btxAttr.TargetTokenID, tx.UnitID())
		}
		if !bytes.Equal(btxAttr.TargetTokenBacklink, attr.Backlink) {
			return 0, fmt.Errorf("burn tx target token backlink does not match with join transaction backlink: burnTx %X, joinTx %X", btxAttr.TargetTokenBacklink, attr.Backlink)
		}
		if err = types.VerifyTxProof(proofs[i], btx, m.trustBase, m.hashAlgorithm); err != nil {
			return 0, fmt.Errorf("proof is not valid: %w", err)
		}
	}
	if !bytes.Equal(d.Backlink, attr.Backlink) {
		return 0, fmt.Errorf("invalid backlink: expected %X, got %X", d.Backlink, attr.Backlink)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx); err != nil {
		return 0, fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
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
		return 0, fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return sum, nil
}
