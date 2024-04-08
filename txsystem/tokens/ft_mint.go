package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *FungibleTokensModule) handleMintFungibleTokenTx() txsystem.GenericExecuteFunc[MintFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateMintFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid mint fungible token tx: %w", err)
		}
		fee := m.feeCalculator()
		typeID := tx.UnitID()
		newTokenID := NewFungibleTokenID(typeID, HashForIDCalculation(tx, m.hashAlgorithm))

		if err := m.state.Apply(
			state.AddUnit(newTokenID, attr.Bearer, newFungibleTokenData(typeID, attr.Value, exeCtx.currentBlockNr, 0, tx.Timeout())),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateMintFungibleToken(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	_, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	if attr.Value == 0 {
		return errors.New("token must have value greater than zero")
	}
	if err = runChainedPredicates[*FungibleTokenTypeData](
		tx,
		tx.UnitID(),
		attr.TokenCreationPredicateSignatures,
		m.execPredicate,
		func(d *FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.TokenCreationPredicate
		},
		m.state.GetUnit,
	); err != nil {
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
		Value:                            m.Value,
		Nonce:                            m.Nonce,
		TokenCreationPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}
