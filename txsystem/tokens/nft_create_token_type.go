package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (n *NonFungibleTokensModule) handleCreateNonFungibleTokenTypeTx() txsystem.GenericExecuteFunc[CreateNonFungibleTokenTypeAttributes] {
	return func(tx *types.TransactionOrder, attr *CreateNonFungibleTokenTypeAttributes, ctx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := n.validate(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid create non-fungible token type tx: %w", err)
		}
		fee := n.feeCalculator()

		// update state
		unitID := tx.UnitID()
		if err := n.state.Apply(
			state.AddUnit(unitID, templates.AlwaysTrueBytes(), newNonFungibleTokenTypeData(attr)),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (n *NonFungibleTokensModule) validate(tx *types.TransactionOrder, attr *CreateNonFungibleTokenTypeAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(NonFungibleTokenTypeUnitType) {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidUnitID)
	}
	if attr.ParentTypeID != nil && !attr.ParentTypeID.HasType(NonFungibleTokenTypeUnitType) {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidParentTypeID)
	}
	if len(attr.Symbol) > maxSymbolLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidSymbolLength)
	}
	if len(attr.Name) > maxNameLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidNameLength)
	}
	if attr.Icon != nil {
		if len(attr.Icon.Type) > maxIconTypeLength {
			return fmt.Errorf("create nft type: %s", ErrStrInvalidIconTypeLength)
		}
		if len(attr.Icon.Data) > maxIconDataLength {
			return fmt.Errorf("create nft type: %s", ErrStrInvalidIconDataLength)
		}
	}
	u, err := n.state.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("create nft type: unit %v exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}

	err = runChainedPredicates[*NonFungibleTokenTypeData](
		tx,
		attr.ParentTypeID,
		attr.SubTypeCreationPredicateSignatures,
		n.execPredicate,
		func(d *NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.SubTypeCreationPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type SubTypeCreationPredicate: %w", err)
	}
	return nil
}

func (c *CreateNonFungibleTokenTypeAttributes) GetSymbol() string {
	return c.Symbol
}

func (c *CreateNonFungibleTokenTypeAttributes) SetSymbol(symbol string) {
	c.Symbol = symbol
}

func (c *CreateNonFungibleTokenTypeAttributes) GetName() string {
	return c.Name
}

func (c *CreateNonFungibleTokenTypeAttributes) SetName(name string) {
	c.Name = name
}

func (c *CreateNonFungibleTokenTypeAttributes) GetIcon() *Icon {
	return c.Icon
}

func (c *CreateNonFungibleTokenTypeAttributes) SetIcon(icon *Icon) {
	c.Icon = icon
}

func (c *CreateNonFungibleTokenTypeAttributes) GetParentTypeID() types.UnitID {
	return c.ParentTypeID
}

func (c *CreateNonFungibleTokenTypeAttributes) SetParentTypeID(parentTypeID types.UnitID) {
	c.ParentTypeID = parentTypeID
}

func (c *CreateNonFungibleTokenTypeAttributes) GetSubTypeCreationPredicate() []byte {
	return c.SubTypeCreationPredicate
}

func (c *CreateNonFungibleTokenTypeAttributes) SetSubTypeCreationPredicate(predicate []byte) {
	c.SubTypeCreationPredicate = predicate
}

func (c *CreateNonFungibleTokenTypeAttributes) GetTokenCreationPredicate() []byte {
	return c.TokenCreationPredicate
}

func (c *CreateNonFungibleTokenTypeAttributes) SetTokenCreationPredicate(predicate []byte) {
	c.TokenCreationPredicate = predicate
}

func (c *CreateNonFungibleTokenTypeAttributes) GetInvariantPredicate() []byte {
	return c.InvariantPredicate
}

func (c *CreateNonFungibleTokenTypeAttributes) SetInvariantPredicate(predicate []byte) {
	c.InvariantPredicate = predicate
}

func (c *CreateNonFungibleTokenTypeAttributes) GetDataUpdatePredicate() []byte {
	return c.DataUpdatePredicate
}

func (c *CreateNonFungibleTokenTypeAttributes) SetDataUpdatePredicate(predicate []byte) {
	c.DataUpdatePredicate = predicate
}

func (c *CreateNonFungibleTokenTypeAttributes) GetSubTypeCreationPredicateSignatures() [][]byte {
	return c.SubTypeCreationPredicateSignatures
}

func (c *CreateNonFungibleTokenTypeAttributes) SetSubTypeCreationPredicateSignatures(signatures [][]byte) {
	c.SubTypeCreationPredicateSignatures = signatures
}

func (c *CreateNonFungibleTokenTypeAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude SubTypeCreationPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &CreateNonFungibleTokenTypeAttributes{
		Symbol:                             c.Symbol,
		Name:                               c.Name,
		ParentTypeID:                       c.ParentTypeID,
		SubTypeCreationPredicate:           c.SubTypeCreationPredicate,
		TokenCreationPredicate:             c.TokenCreationPredicate,
		InvariantPredicate:                 c.InvariantPredicate,
		DataUpdatePredicate:                c.DataUpdatePredicate,
		Icon:                               c.Icon,
		SubTypeCreationPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}
