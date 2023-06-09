package tokens

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleCreateNoneFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[CreateNonFungibleTokenTypeAttributes] {
	return func(tx *types.TransactionOrder, attr *CreateNonFungibleTokenTypeAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Create Non-Fungible Token Type tx: %v", tx)
		if err := validate(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid create none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.AddItem(unitID, script.PredicateAlwaysTrue(), newNonFungibleTokenTypeData(attr), h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validate(tx *types.TransactionOrder, attr *CreateNonFungibleTokenTypeAttributes, state *rma.Tree) error {
	unitID := util.BytesToUint256(tx.UnitID())
	if unitID.IsZero() {
		return fmt.Errorf("create nft type: %s", ErrStrUnitIDIsZero)
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
	u, err := state.GetUnit(unitID)
	if u != nil {
		return fmt.Errorf("create nft type: unit %v exists", unitID)
	}
	if !errors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	// signature satisfies the predicate obtained by concatenating all the
	// sub-type creation clauses along the type inheritance chain.
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		state,
		util.BytesToUint256(attr.ParentTypeID),
		func(d *nonFungibleTokenTypeData) []byte {
			return d.subTypeCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
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

func (c *CreateNonFungibleTokenTypeAttributes) GetParentTypeID() []byte {
	return c.ParentTypeID
}

func (c *CreateNonFungibleTokenTypeAttributes) SetParentTypeID(parentTypeID []byte) {
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
	return cbor.Marshal(signatureAttr)
}
