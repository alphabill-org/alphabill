package tokens

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

func handleCreateFungibleTokenTypeTx(options *Options) txsystem.GenericExecuteFunc[CreateFungibleTokenTypeAttributes] {
	return func(tx *types.TransactionOrder, attr *CreateFungibleTokenTypeAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := validateCreateFungibleTokenType(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid create fungible token type tx: %w", err)
		}
		fee := options.feeCalculator()

		unitID := tx.UnitID()
		// update state
		if err := options.state.Apply(
			state.AddUnit(unitID, templates.AlwaysTrueBytes(), newFungibleTokenTypeData(attr)),
		); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateCreateFungibleTokenType(tx *types.TransactionOrder, attr *CreateFungibleTokenTypeAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	unitID := tx.UnitID()
	if !unitID.HasType(FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	if attr.ParentTypeID != nil && !attr.ParentTypeID.HasType(FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidParentTypeID)
	}
	if len(attr.Symbol) > maxSymbolLength {
		return errors.New(ErrStrInvalidSymbolLength)
	}
	if len(attr.Name) > maxNameLength {
		return errors.New(ErrStrInvalidNameLength)
	}
	if attr.Icon != nil {
		if len(attr.Icon.Type) > maxIconTypeLength {
			return errors.New(ErrStrInvalidIconTypeLength)
		}
		if len(attr.Icon.Data) > maxIconDataLength {
			return errors.New(ErrStrInvalidIconDataLength)
		}
	}

	decimalPlaces := attr.DecimalPlaces
	if decimalPlaces > maxDecimalPlaces {
		return fmt.Errorf("invalid decimal places. maximum allowed value %v, got %v", maxDecimalPlaces, decimalPlaces)
	}

	u, err := s.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("unit %v exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}

	if attr.ParentTypeID != nil {
		_, parentData, err := getUnit[*FungibleTokenTypeData](s, attr.ParentTypeID)
		if err != nil {
			return err
		}
		if decimalPlaces != parentData.DecimalPlaces {
			return fmt.Errorf("invalid decimal places. allowed %v, got %v", parentData.DecimalPlaces, decimalPlaces)
		}
	}

	predicates, err := getChainedPredicates[*FungibleTokenTypeData](
		hashAlgorithm,
		s,
		attr.ParentTypeID,
		func(d *FungibleTokenTypeData) []byte {
			return d.SubTypeCreationPredicate
		},
		func(d *FungibleTokenTypeData) types.UnitID {
			return d.ParentTypeId
		},
	)
	if err != nil {
		return err
	}

	sigBytes, err := tx.Payload.BytesWithAttributeSigBytes(attr)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, attr.SubTypeCreationPredicateSignatures, sigBytes)
}

func (c *CreateFungibleTokenTypeAttributes) GetSymbol() string {
	return c.Symbol
}

func (c *CreateFungibleTokenTypeAttributes) SetSymbol(symbol string) {
	c.Symbol = symbol
}

func (c *CreateFungibleTokenTypeAttributes) GetName() string {
	return c.Name
}

func (c *CreateFungibleTokenTypeAttributes) SetName(name string) {
	c.Name = name
}

func (c *CreateFungibleTokenTypeAttributes) GetIcon() *Icon {
	return c.Icon
}

func (c *CreateFungibleTokenTypeAttributes) SetIcon(icon *Icon) {
	c.Icon = icon
}

func (c *CreateFungibleTokenTypeAttributes) GetParentTypeID() types.UnitID {
	return c.ParentTypeID
}

func (c *CreateFungibleTokenTypeAttributes) SetParentTypeID(parentTypeID types.UnitID) {
	c.ParentTypeID = parentTypeID
}

func (c *CreateFungibleTokenTypeAttributes) GetDecimalPlaces() uint32 {
	return c.DecimalPlaces
}

func (c *CreateFungibleTokenTypeAttributes) SetDecimalPlaces(decimalPlaces uint32) {
	c.DecimalPlaces = decimalPlaces
}

func (c *CreateFungibleTokenTypeAttributes) GetSubTypeCreationPredicate() []byte {
	return c.SubTypeCreationPredicate
}

func (c *CreateFungibleTokenTypeAttributes) SetSubTypeCreationPredicate(predicate []byte) {
	c.SubTypeCreationPredicate = predicate
}

func (c *CreateFungibleTokenTypeAttributes) GetTokenCreationPredicate() []byte {
	return c.TokenCreationPredicate
}

func (c *CreateFungibleTokenTypeAttributes) SetTokenCreationPredicate(predicate []byte) {
	c.TokenCreationPredicate = predicate
}

func (c *CreateFungibleTokenTypeAttributes) GetInvariantPredicate() []byte {
	return c.InvariantPredicate
}

func (c *CreateFungibleTokenTypeAttributes) SetInvariantPredicate(predicate []byte) {
	c.InvariantPredicate = predicate
}

func (c *CreateFungibleTokenTypeAttributes) GetSubTypeCreationPredicateSignatures() [][]byte {
	return c.SubTypeCreationPredicateSignatures
}

func (c *CreateFungibleTokenTypeAttributes) SetSubTypeCreationPredicateSignatures(signatures [][]byte) {
	c.SubTypeCreationPredicateSignatures = signatures
}

func (c *CreateFungibleTokenTypeAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude SubTypeCreationPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &CreateFungibleTokenTypeAttributes{
		Symbol:                             c.Symbol,
		Name:                               c.Name,
		Icon:                               c.Icon,
		ParentTypeID:                       c.ParentTypeID,
		DecimalPlaces:                      c.DecimalPlaces,
		SubTypeCreationPredicate:           c.SubTypeCreationPredicate,
		TokenCreationPredicate:             c.TokenCreationPredicate,
		InvariantPredicate:                 c.InvariantPredicate,
		SubTypeCreationPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
