package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *FungibleTokensModule) handleMintFungibleTokenTx() txsystem.GenericExecuteFunc[MintFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes, ctx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateMintFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid mint fungible token tx: %w", err)
		}
		fee := m.feeCalculator()

		unitID := tx.UnitID()
		h := tx.Hash(m.hashAlgorithm)

		// update state
		if err := m.state.Apply(
			state.AddUnit(unitID, attr.Bearer, newFungibleTokenData(attr, h, ctx.CurrentBlockNr)),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateMintFungibleToken(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(FungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := m.state.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("unit with id %v already exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if attr.Value == 0 {
		return errors.New("token must have value greater than zero")
	}
	if !attr.TypeID.HasType(FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidTypeID)
	}

	err = runChainedPredicates[*FungibleTokenTypeData](
		tx,
		attr.TypeID,
		attr.TokenCreationPredicateSignatures,
		m.execPredicate,
		func(d *FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.TokenCreationPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("evaluating TokenCreationPredicate: %w", err)
	}
	return nil
}

func (m *MintFungibleTokenAttributes) GetBearer() []byte {
	return m.Bearer
}

func (m *MintFungibleTokenAttributes) SetBearer(bearer []byte) {
	m.Bearer = bearer
}

func (m *MintFungibleTokenAttributes) GetTypeID() types.UnitID {
	return m.TypeID
}

func (m *MintFungibleTokenAttributes) SetTypeID(typeID types.UnitID) {
	m.TypeID = typeID
}

func (m *MintFungibleTokenAttributes) GetValue() uint64 {
	return m.Value
}

func (m *MintFungibleTokenAttributes) SetValue(value uint64) {
	m.Value = value
}

func (m *MintFungibleTokenAttributes) GetTokenCreationPredicateSignatures() [][]byte {
	return m.TokenCreationPredicateSignatures
}

func (m *MintFungibleTokenAttributes) SetTokenCreationPredicateSignatures(signatures [][]byte) {
	m.TokenCreationPredicateSignatures = signatures
}

func (m *MintFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016
	signatureAttr := &MintFungibleTokenAttributes{
		Bearer:                           m.Bearer,
		TypeID:                           m.TypeID,
		Value:                            m.Value,
		TokenCreationPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}
