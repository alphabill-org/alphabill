package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *FungibleTokensModule) handleSplitFungibleTokenTx() txsystem.GenericExecuteFunc[SplitFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateSplitFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid split fungible token tx: %w", err)
		}
		unitID := tx.UnitID()
		u, err := m.state.GetUnit(unitID, false)
		if err != nil {
			return nil, err
		}
		ftData := u.Data().(*FungibleTokenData)

		// add new token unit
		newTokenID := NewFungibleTokenID(unitID, HashForIDCalculation(tx, m.hashAlgorithm))
		fee := m.feeCalculator()

		// update state
		if err := m.state.Apply(
			state.AddUnit(newTokenID,
				attr.NewBearer,
				&FungibleTokenData{
					TokenType: ftData.TokenType,
					Value:     attr.TargetValue,
					T:         exeCtx.CurrentBlockNr,
					Counter:   0,
					T1:        0,
					Locked:    0,
				}),
			state.UpdateUnitData(unitID,
				func(data state.UnitData) (state.UnitData, error) {
					d, ok := data.(*FungibleTokenData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
					}
					return &FungibleTokenData{
						TokenType: d.TokenType,
						Value:     d.Value - attr.TargetValue,
						T:         exeCtx.CurrentBlockNr,
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
}

func HashForIDCalculation(tx *types.TransactionOrder, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	hasher.Write(tx.UnitID())
	hasher.Write(tx.Payload.Attributes)
	tx.Payload.ClientMetadata.AddToHasher(hasher)
	return hasher.Sum(nil)
}

func (m *FungibleTokensModule) validateSplitFungibleToken(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes) error {
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

	if d.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", d.Counter, attr.Counter)
	}
	if !bytes.Equal(attr.TypeID, d.TokenType) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", d.TokenType, attr.TypeID)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err = runChainedPredicates[*FungibleTokenTypeData](
		tx,
		d.TokenType,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("evaluating InvariantPredicate: %w", err)
	}
	return nil
}

func (s *SplitFungibleTokenAttributes) GetNewBearer() []byte {
	return s.NewBearer
}

func (s *SplitFungibleTokenAttributes) SetNewBearer(newBearer []byte) {
	s.NewBearer = newBearer
}

func (s *SplitFungibleTokenAttributes) GetTargetValue() uint64 {
	return s.TargetValue
}

func (s *SplitFungibleTokenAttributes) SetTargetValue(targetValue uint64) {
	s.TargetValue = targetValue
}

func (s *SplitFungibleTokenAttributes) GetNonce() []byte {
	return s.Nonce
}

func (s *SplitFungibleTokenAttributes) SetNonce(nonce []byte) {
	s.Nonce = nonce
}

func (s *SplitFungibleTokenAttributes) GetCounter() uint64 {
	return s.Counter
}

func (s *SplitFungibleTokenAttributes) SetCounter(counter uint64) {
	s.Counter = counter
}

func (s *SplitFungibleTokenAttributes) GetTypeID() types.UnitID {
	return s.TypeID
}

func (s *SplitFungibleTokenAttributes) SetTypeID(typeID types.UnitID) {
	s.TypeID = typeID
}

func (s *SplitFungibleTokenAttributes) GetRemainingValue() uint64 {
	return s.RemainingValue
}

func (s *SplitFungibleTokenAttributes) SetRemainingValue(remainingValue uint64) {
	s.RemainingValue = remainingValue
}

func (s *SplitFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return s.InvariantPredicateSignatures
}

func (s *SplitFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	s.InvariantPredicateSignatures = signatures
}

func (s *SplitFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &SplitFungibleTokenAttributes{
		NewBearer:                    s.NewBearer,
		TargetValue:                  s.TargetValue,
		Nonce:                        s.Nonce,
		Counter:                      s.Counter,
		TypeID:                       s.TypeID,
		RemainingValue:               s.RemainingValue,
		InvariantPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}
