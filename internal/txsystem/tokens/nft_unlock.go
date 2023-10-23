package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
)

func handleUnlockNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[UnlockNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UnlockNonFungibleTokenAttributes, roundNumber uint64) (*types.ServerMetadata, error) {
		if err := validateUnlockNonFungibleTokenTx(tx, attr, options); err != nil {
			return nil, fmt.Errorf("invalid unlock non-fungible token tx: %w", err)
		}
		// update lock status, round number and backlink
		updateFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				d, ok := data.(*nonFungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain non-fungible token data", tx.UnitID())
				}
				d.t = roundNumber
				d.backlink = tx.Hash(options.hashAlgorithm)
				d.locked = 0
				return d, nil
			})
		if err := options.state.Apply(updateFn); err != nil {
			return nil, fmt.Errorf("failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: options.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func validateUnlockNonFungibleTokenTx(tx *types.TransactionOrder, attr *UnlockNonFungibleTokenAttributes, options *Options) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if attr == nil {
		return errors.New("attributes is nil")
	}
	if options == nil {
		return errors.New("options is nil")
	}
	// unit id identifies an existing fungible token
	bearer, tokenData, err := getNonFungibleTokenData(tx.UnitID(), options.state)
	if err != nil {
		return err
	}
	// the token is locked
	if tokenData.locked == 0 {
		return errors.New("token is already unlocked")
	}
	// the current transaction follows the previous valid transaction with the token
	if !bytes.Equal(attr.Backlink, tokenData.backlink) {
		return fmt.Errorf("the transaction backlink is not equal to the token backlink: tx.backlink='%x' token.backlink='%x'",
			attr.Backlink, tokenData.backlink)
	}
	// the InvariantPredicateSignatures in the transaction request satisfies the multipart predicate obtained
	// by joining all the inherited bearer clauses along the type inheritance chain.
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		options.hashAlgorithm,
		options.state,
		tokenData.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *nonFungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
		},
	)
	if err != nil {
		return fmt.Errorf("failed to load token type predicate chain: %w", err)
	}
	return verifyOwnership(bearer, predicates, &unlockNonFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

type unlockNonFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *UnlockNonFungibleTokenAttributes
}

func (t *unlockNonFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *unlockNonFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *unlockNonFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
}

func (l *UnlockNonFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &UnlockNonFungibleTokenAttributes{
		Backlink:                     l.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
