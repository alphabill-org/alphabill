package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
)

func handleBurnFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[BurnFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *BurnFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := validateBurnFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid burn fungible token transaction: %w", err)
		}
		fee := options.feeCalculator()

		// update state
		unitID := tx.UnitID()
		if err := options.state.Apply(state.DeleteUnit(unitID)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateBurnFungibleToken(tx *types.TransactionOrder, attr *BurnFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), s)
	if err != nil {
		return err
	}
	if d.locked != 0 {
		return errors.New("token is locked")
	}
	if !bytes.Equal(d.tokenType, attr.TypeID) {
		return fmt.Errorf("type of token to burn does not matches the actual type of the token: expected %s, got %s", d.tokenType, attr.TypeID)
	}
	if attr.Value != d.value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.value, attr.Value)
	}
	if !bytes.Equal(d.backlink, attr.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		hashAlgorithm,
		s,
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyOwnership(bearer, predicates, &burnFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

type burnFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *BurnFungibleTokenAttributes
}

func (t *burnFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *burnFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *burnFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
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
	return cbor.Marshal(signatureAttr)
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
