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

func handleLockFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[LockFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *LockFungibleTokenAttributes, roundNumber uint64) (*types.ServerMetadata, error) {
		if err := validateLockFungibleTokenTx(tx, attr, options); err != nil {
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
				d.locked = attr.LockStatus
				return d, nil
			})
		if err := options.state.Apply(updateFn); err != nil {
			return nil, fmt.Errorf("failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: options.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func validateLockFungibleTokenTx(tx *types.TransactionOrder, attr *LockFungibleTokenAttributes, options *Options) error {
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
	// the token is not locked
	if tokenData.locked != 0 {
		return errors.New("token is already locked")
	}
	// the new status is a "locked" one
	if attr.LockStatus == 0 {
		return errors.New("lock status cannot be zero-value")
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
	return verifyOwnership(bearer, predicates, &lockFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

type lockFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *LockFungibleTokenAttributes
}

func (t *lockFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *lockFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *lockFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
}

func (l *LockFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &LockFungibleTokenAttributes{
		LockStatus:                   l.LockStatus,
		Backlink:                     l.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
