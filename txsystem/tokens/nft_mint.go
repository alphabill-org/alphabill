package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

func (n *NonFungibleTokensModule) handleMintNonFungibleTokenTx() txsystem.GenericExecuteFunc[MintNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *MintNonFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := n.validateMintNonFungibleToken(tx, attr, exeCtx); err != nil {
			return nil, fmt.Errorf("invalid mint non-fungible token tx: %w", err)
		}
		fee := n.feeCalculator()
		typeID := tx.UnitID()
		newTokenID := NewNonFungibleTokenID(typeID, HashForIDCalculation(tx, n.hashAlgorithm))

		if err := n.state.Apply(
			state.AddUnit(newTokenID, attr.Bearer, newNonFungibleTokenData(typeID, attr, exeCtx.CurrentBlockNr, 0)),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{newTokenID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (n *NonFungibleTokensModule) validateMintNonFungibleToken(tx *types.TransactionOrder, attr *MintNonFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) error {
	unitID := tx.UnitID()
	if !unitID.HasType(NonFungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	if len(attr.Name) > maxNameLength {
		return errors.New(ErrStrInvalidNameLength)
	}
	uri := attr.URI
	if uri != "" {
		if len(uri) > uriMaxSize {
			return fmt.Errorf("URI exceeds the maximum allowed size of %v KB", uriMaxSize)
		}
		if !util.IsValidURI(uri) {
			return fmt.Errorf("URI %s is invalid", uri)
		}
	}
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	_, err := n.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}

	err = runChainedPredicates[*NonFungibleTokenTypeData](
		exeCtx,
		tx,
		unitID,
		attr.TokenCreationPredicateSignatures,
		n.execPredicate,
		func(d *NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.TokenCreationPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf(`executing NFT type's "TokenCreationPredicate": %w`, err)
	}
	return nil
}

func (m *MintNonFungibleTokenAttributes) GetBearer() []byte {
	return m.Bearer
}

func (m *MintNonFungibleTokenAttributes) SetBearer(bearer []byte) {
	m.Bearer = bearer
}

func (m *MintNonFungibleTokenAttributes) GetName() string {
	return m.Name
}

func (m *MintNonFungibleTokenAttributes) SetName(name string) {
	m.Name = name
}

func (m *MintNonFungibleTokenAttributes) GetURI() string {
	return m.URI
}

func (m *MintNonFungibleTokenAttributes) SetURI(uri string) {
	m.URI = uri
}

func (m *MintNonFungibleTokenAttributes) GetData() []byte {
	return m.Data
}

func (m *MintNonFungibleTokenAttributes) SetData(data []byte) {
	m.Data = data
}

func (m *MintNonFungibleTokenAttributes) GetDataUpdatePredicate() []byte {
	return m.DataUpdatePredicate
}

func (m *MintNonFungibleTokenAttributes) SetDataUpdatePredicate(predicate []byte) {
	m.DataUpdatePredicate = predicate
}

func (m *MintNonFungibleTokenAttributes) GetTokenCreationPredicateSignatures() [][]byte {
	return m.TokenCreationPredicateSignatures
}

func (m *MintNonFungibleTokenAttributes) SetTokenCreationPredicateSignatures(signatures [][]byte) {
	m.TokenCreationPredicateSignatures = signatures
}

func (m *MintNonFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude TokenCreationPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &MintNonFungibleTokenAttributes{
		Bearer:                           m.Bearer,
		Name:                             m.Name,
		URI:                              m.URI,
		Data:                             m.Data,
		DataUpdatePredicate:              m.DataUpdatePredicate,
		Nonce:                            m.Nonce,
		TokenCreationPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}
