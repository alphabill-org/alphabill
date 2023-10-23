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

func handleUnlockFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[UnlockFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UnlockFungibleTokenAttributes, roundNumber uint64) (*types.ServerMetadata, error) {
		if err := validateUnlockFungibleTokenTx(tx, attr, options); err != nil {
			return nil, fmt.Errorf("invalid lock fungible token tx: %w", err)
		}
		// update lock status, round number and backlink
		updateFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				d, ok := data.(*fungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
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

func validateUnlockFungibleTokenTx(tx *types.TransactionOrder, attr *UnlockFungibleTokenAttributes, options *Options) error {
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
	bearer, tokenData, err := getFungibleTokenData(tx.UnitID(), options.state)
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
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		options.hashAlgorithm,
		options.state,
		tokenData.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
		},
	)
	if err != nil {
		return fmt.Errorf("failed to load token type predicate chain: %w", err)
	}
	return verifyOwnership(bearer, predicates, &unlockFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

type unlockFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *UnlockFungibleTokenAttributes
}

func (t *unlockFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *unlockFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *unlockFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
}

func (l *UnlockFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &UnlockFungibleTokenAttributes{
		Backlink:                     l.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
