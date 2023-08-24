package tokens

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/tree/avl"
	"github.com/fxamacker/cbor/v2"
)

var ErrStrUnitIDIsZero = "unit ID cannot be zero"

func handleCreateFungibleTokenTypeTx(options *Options) txsystem.GenericExecuteFunc[CreateFungibleTokenTypeAttributes] {
	return func(tx *types.TransactionOrder, attr *CreateFungibleTokenTypeAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Create Fungible Token Type tx: %v", tx)
		if err := validateCreateFungibleTokenType(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid create fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		unitID := tx.UnitID()
		sm := &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}}
		// update state
		if err := options.state.Apply(
			state.AddUnit(unitID, script.PredicateAlwaysTrue(), newFungibleTokenTypeData(attr)),
		); err != nil {
			return nil, err
		}

		return sm, nil
	}
}

func validateCreateFungibleTokenType(tx *types.TransactionOrder, attr *CreateFungibleTokenTypeAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	unitID := types.UnitID(tx.UnitID())
	if unitID.IsZero(UnitPartLength) {
		return errors.New(ErrStrUnitIDIsZero)
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

	parentUnitID := types.UnitID(attr.ParentTypeID)
	if !parentUnitID.IsZero(UnitPartLength) {
		_, parentData, err := getUnit[*fungibleTokenTypeData](s, parentUnitID)
		if err != nil {
			return err
		}
		if decimalPlaces != parentData.decimalPlaces {
			return fmt.Errorf("invalid decimal places. allowed %v, got %v", parentData.decimalPlaces, decimalPlaces)
		}
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		hashAlgorithm,
		s,
		attr.ParentTypeID,
		func(d *fungibleTokenTypeData) []byte {
			return d.subTypeCreationPredicate
		},
		func(d *fungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
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

func (c *CreateFungibleTokenTypeAttributes) GetParentTypeID() []byte {
	return c.ParentTypeID
}

func (c *CreateFungibleTokenTypeAttributes) SetParentTypeID(parentTypeID []byte) {
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
