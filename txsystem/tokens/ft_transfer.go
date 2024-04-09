package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *FungibleTokensModule) handleTransferFungibleTokenTx() txsystem.GenericExecuteFunc[TransferFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateTransferFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid transfer fungible token tx: %w", err)
		}
		fee := m.feeCalculator()
		unitID := tx.UnitID()

		// update state
		if err := m.state.Apply(
			state.SetOwner(unitID, attr.NewBearer),
			state.UpdateUnitData(unitID,
				func(data state.UnitData) (state.UnitData, error) {
					d, ok := data.(*FungibleTokenData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
					}
					d.T = exeCtx.CurrentBlockNr
					d.Backlink = tx.Hash(m.hashAlgorithm)
					return d, nil
				})); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateTransferFungibleToken(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return err
	}

	if d.Locked != 0 {
		return fmt.Errorf("token is locked")
	}

	if d.Value != attr.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.Value, attr.Value)
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

func getFungibleTokenData(unitID types.UnitID, s *state.State) (types.PredicateBytes, *FungibleTokenData, error) {
	if !unitID.HasType(FungibleTokenUnitType) {
		return nil, nil, fmt.Errorf(ErrStrInvalidUnitID)
	}

	u, err := s.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return nil, nil, fmt.Errorf("unit %v does not exist: %w", unitID, err)
		}
		return nil, nil, err
	}
	d, ok := u.Data().(*FungibleTokenData)
	if !ok {
		return nil, nil, fmt.Errorf("unit %v is not fungible token data", unitID)
	}
	return u.Bearer(), d, nil
}

func (t *TransferFungibleTokenAttributes) GetNewBearer() []byte {
	return t.NewBearer
}

func (t *TransferFungibleTokenAttributes) SetNewBearer(newBearer []byte) {
	t.NewBearer = newBearer
}

func (t *TransferFungibleTokenAttributes) GetValue() uint64 {
	return t.Value
}

func (t *TransferFungibleTokenAttributes) SetValue(value uint64) {
	t.Value = value
}

func (t *TransferFungibleTokenAttributes) GetNonce() []byte {
	return t.Nonce
}

func (t *TransferFungibleTokenAttributes) SetNonce(nonce []byte) {
	t.Nonce = nonce
}

func (t *TransferFungibleTokenAttributes) GetBacklink() []byte {
	return t.Backlink
}

func (t *TransferFungibleTokenAttributes) SetBacklink(backlink []byte) {
	t.Backlink = backlink
}

func (t *TransferFungibleTokenAttributes) GetTypeID() types.UnitID {
	return t.TypeID
}

func (t *TransferFungibleTokenAttributes) SetTypeID(typeID types.UnitID) {
	t.TypeID = typeID
}

func (t *TransferFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return t.InvariantPredicateSignatures
}

func (t *TransferFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	t.InvariantPredicateSignatures = signatures
}

func (t *TransferFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &TransferFungibleTokenAttributes{
		NewBearer:                    t.NewBearer,
		Value:                        t.Value,
		Nonce:                        t.Nonce,
		Backlink:                     t.Backlink,
		TypeID:                       t.TypeID,
		InvariantPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}
