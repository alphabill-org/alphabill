package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/pkg/tree/avl"
	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
)

func handleLockTokenTx(options *Options) txsystem.GenericExecuteFunc[LockTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64) (*types.ServerMetadata, error) {
		if err := validateLockTokenTx(tx, attr, options); err != nil {
			return nil, fmt.Errorf("invalid lock token tx: %w", err)
		}
		updateFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				return updateLockTokenData(data, tx, attr, roundNumber, options)
			})
		if err := options.state.Apply(updateFn); err != nil {
			return nil, fmt.Errorf("failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: options.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func updateLockTokenData(data state.UnitData, tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64, options *Options) (state.UnitData, error) {
	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return updateLockFungibleTokenData(data, tx, attr, roundNumber, options)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return updateLockNonFungibleTokenData(data, tx, attr, roundNumber, options)
	} else {
		return nil, fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func updateLockNonFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64, options *Options) (state.UnitData, error) {
	d, ok := data.(*nonFungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.t = roundNumber
	d.backlink = tx.Hash(options.hashAlgorithm)
	d.locked = attr.LockStatus
	return d, nil
}

func updateLockFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64, options *Options) (state.UnitData, error) {
	d, ok := data.(*fungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.t = roundNumber
	d.backlink = tx.Hash(options.hashAlgorithm)
	d.locked = attr.LockStatus
	return d, nil
}

func validateLockTokenTx(tx *types.TransactionOrder, attr *LockTokenAttributes, options *Options) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if attr == nil {
		return errors.New("attributes is nil")
	}
	if options == nil {
		return errors.New("options is nil")
	}

	// unit id identifies an existing fungible or non-fungible token
	u, err := options.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return fmt.Errorf("unit '%v' does not exist: %w", tx.UnitID(), err)
		}
		return err
	}

	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return validateFungibleLockToken(tx, attr, options, u)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return validateNonFungibleLockToken(tx, attr, options, u)
	} else {
		return fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func validateFungibleLockToken(tx *types.TransactionOrder, attr *LockTokenAttributes, options *Options, u *state.Unit) error {
	d, ok := u.Data().(*fungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not fungible token data", tx.UnitID())
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		options.hashAlgorithm,
		options.state,
		d.tokenType,
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
	return validateTokenLock(u, tx, attr, predicates, d)
}

func validateNonFungibleLockToken(tx *types.TransactionOrder, attr *LockTokenAttributes, options *Options, u *state.Unit) error {
	d, ok := u.Data().(*nonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not non-fungible token data", tx.UnitID())
	}
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		options.hashAlgorithm,
		options.state,
		d.nftType,
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
	return validateTokenLock(u, tx, attr, predicates, d)
}

type lockTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *LockTokenAttributes
}

func (t *lockTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *lockTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *lockTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
}

func (l *LockTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &LockTokenAttributes{
		LockStatus:                   l.LockStatus,
		Backlink:                     l.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}

type tokenData interface {
	Backlink() []byte
	Locked() uint64
}

func validateTokenLock(u *state.Unit, tx *types.TransactionOrder, attr *LockTokenAttributes, predicates []state.Predicate, d tokenData) error {
	// token is not locked
	if d.Locked() != 0 {
		return errors.New("token is already locked")
	}
	// the new status is a "locked" one
	if attr.LockStatus == 0 {
		return errors.New("lock status cannot be zero-value")
	}
	// the current transaction follows the previous valid transaction with the token
	if !bytes.Equal(attr.Backlink, d.Backlink()) {
		return fmt.Errorf("the transaction backlink is not equal to the token backlink: tx.backlink='%x' token.backlink='%x'",
			attr.Backlink, d.Backlink())
	}
	return verifyOwnership(u.Bearer(), predicates, &lockTokenOwnershipProver{tx: tx, attr: attr})
}
