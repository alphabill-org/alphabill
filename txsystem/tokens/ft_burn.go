package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

var (
	DustCollectorPredicate = templates.NewP2pkh256BytesFromKeyHash(hash.Sum256([]byte("dust collector")))
)

func (m *FungibleTokensModule) handleBurnFungibleTokenTx() txsystem.GenericExecuteFunc[BurnFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *BurnFungibleTokenAttributes, currentBlockNo uint64) (*types.ServerMetadata, error) {
		if err := m.validateBurnFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid burn fungible token transaction: %w", err)
		}
		fee := m.feeCalculator()
		unitID := tx.UnitID()
		txHash := tx.Hash(m.hashAlgorithm)

		// 1. SetOwner(ι, DC)
		setOwnerFn := state.SetOwner(unitID, DustCollectorPredicate)

		// 2. UpdateData(ι, f), where f(D) = (0, S.n, H(P))
		updateUnitFn := state.UpdateUnitData(unitID,
			func(data state.UnitData) (state.UnitData, error) {
				ftData, ok := data.(*FungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
				}
				ftData.Value = 0
				ftData.T = currentBlockNo
				ftData.Backlink = txHash
				return ftData, nil
			},
		)

		if err := m.state.Apply(setOwnerFn, updateUnitFn); err != nil {
			return nil, fmt.Errorf("burnFToken: failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateBurnFungibleToken(tx *types.TransactionOrder, attr *BurnFungibleTokenAttributes) error {
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
	if !bytes.Equal(d.Backlink, attr.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.Backlink, attr.Backlink)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("bearer predicate: %w", err)
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
		return fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return nil
}

func (b *BurnFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &BurnFungibleTokenAttributes{
		TypeID:                       b.TypeID,
		Value:                        b.Value,
		TargetTokenID:                b.TargetTokenID,
		TargetTokenBacklink:          b.TargetTokenBacklink,
		Backlink:                     b.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}

func (b *BurnFungibleTokenAttributes) GetTypeID() types.UnitID {
	return b.TypeID
}

func (b *BurnFungibleTokenAttributes) SetTypeID(typeID types.UnitID) {
	b.TypeID = typeID
}

func (b *BurnFungibleTokenAttributes) GetValue() uint64 {
	return b.Value
}

func (b *BurnFungibleTokenAttributes) SetValue(value uint64) {
	b.Value = value
}

func (b *BurnFungibleTokenAttributes) GetTargetTokenID() []byte {
	return b.TargetTokenID
}

func (b *BurnFungibleTokenAttributes) SetTargetTokenID(targetTokenID []byte) {
	b.TargetTokenID = targetTokenID
}

func (b *BurnFungibleTokenAttributes) GetTargetTokenBacklink() []byte {
	return b.TargetTokenBacklink
}

func (b *BurnFungibleTokenAttributes) SetTargetTokenBacklink(targetTokenBacklink []byte) {
	b.TargetTokenBacklink = targetTokenBacklink
}

func (b *BurnFungibleTokenAttributes) GetBacklink() []byte {
	return b.Backlink
}

func (b *BurnFungibleTokenAttributes) SetBacklink(backlink []byte) {
	b.Backlink = backlink
}

func (b *BurnFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return b.InvariantPredicateSignatures
}

func (b *BurnFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	b.InvariantPredicateSignatures = signatures
}
