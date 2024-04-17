package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *FungibleTokensModule) handleSplitFungibleTokenTx() txsystem.GenericExecuteFunc[tokens.SplitFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := m.validateSplitFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid split fungible token tx: %w", err)
		}
		unitID := tx.UnitID()
		u, err := m.state.GetUnit(unitID, false)
		if err != nil {
			return nil, err
		}
		d := u.Data().(*tokens.FungibleTokenData)
		// add new token unit
		newTokenID := tokens.NewFungibleTokenID(unitID, HashForIDCalculation(tx, m.hashAlgorithm))

		fee := m.feeCalculator()
		txHash := tx.Hash(m.hashAlgorithm)

		// update state
		if err := m.state.Apply(
			state.AddUnit(newTokenID,
				attr.NewBearer,
				&tokens.FungibleTokenData{
					TokenType: d.TokenType,
					Value:     attr.TargetValue,
					T:         currentBlockNr,
					Backlink:  txHash,
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
						T:         currentBlockNr,
						Backlink:  txHash,
					}, nil
				})); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID, newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func HashForIDCalculation(tx *types.TransactionOrder, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	hasher.Write(tx.UnitID())
	hasher.Write(tx.Payload.Attributes)
	hasher.Write(util.Uint64ToBytes(tx.Timeout()))
	return hasher.Sum(nil)
}

func (m *FungibleTokensModule) validateSplitFungibleToken(tx *types.TransactionOrder, attr *tokens.SplitFungibleTokenAttributes) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return err
	}
	if d.Locked != 0 {
		return errors.New("token is locked")
	}
	if attr.TargetValue == 0 {
		return errors.New("when splitting a token the value assigned to the new token must be greater than zero")
	}
	if attr.RemainingValue == 0 {
		return errors.New("when splitting a token the remaining value of the token must be greater than zero")
	}

	if d.Value < attr.TargetValue {
		return fmt.Errorf("invalid token value: max allowed %v, got %v", d.Value, attr.TargetValue)
	}
	if rm := d.Value - attr.TargetValue; attr.RemainingValue != rm {
		return errors.New("remaining value must equal to the original value minus target value")
	}

	if !bytes.Equal(d.Backlink, attr.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.Backlink, attr.Backlink)
	}
	if !bytes.Equal(attr.TypeID, d.TokenType) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", d.TokenType, attr.TypeID)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
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
		return fmt.Errorf("evaluating InvariantPredicate: %w", err)
	}
	return nil
}
