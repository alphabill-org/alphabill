package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
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
	d, ok := data.(*NonFungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Backlink = tx.Hash(options.hashAlgorithm)
	d.Locked = attr.LockStatus
	return d, nil
}

func updateLockFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64, options *Options) (state.UnitData, error) {
	d, ok := data.(*FungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Backlink = tx.Hash(options.hashAlgorithm)
	d.Locked = attr.LockStatus
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
	d, ok := u.Data().(*FungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not fungible token data", tx.UnitID())
	}
	predicates, err := getChainedPredicates[*FungibleTokenTypeData](
		options.hashAlgorithm,
		options.state,
		d.TokenType,
		func(d *FungibleTokenTypeData) []byte {
			return d.InvariantPredicate
		},
		func(d *FungibleTokenTypeData) types.UnitID {
			return d.ParentTypeId
		},
	)
	if err != nil {
		return fmt.Errorf("failed to load token type predicate chain: %w", err)
	}
	return validateTokenLock(u, tx, attr, predicates, d)
}

func validateNonFungibleLockToken(tx *types.TransactionOrder, attr *LockTokenAttributes, options *Options, u *state.Unit) error {
	d, ok := u.Data().(*NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not non-fungible token data", tx.UnitID())
	}
	predicates, err := getChainedPredicates[*NonFungibleTokenTypeData](
		options.hashAlgorithm,
		options.state,
		d.NftType,
		func(d *NonFungibleTokenTypeData) []byte {
			return d.InvariantPredicate
		},
		func(d *NonFungibleTokenTypeData) types.UnitID {
			return d.ParentTypeId
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
	GetBacklink() []byte
	IsLocked() uint64
}

func validateTokenLock(u *state.Unit, tx *types.TransactionOrder, attr *LockTokenAttributes, predicates []predicates.PredicateBytes, d tokenData) error {
	// token is not locked
	if d.IsLocked() != 0 {
		return errors.New("token is already locked")
	}
	// the new status is a "locked" one
	if attr.LockStatus == 0 {
		return errors.New("lock status cannot be zero-value")
	}
	// the current transaction follows the previous valid transaction with the token
	if !bytes.Equal(attr.Backlink, d.GetBacklink()) {
		return fmt.Errorf("the transaction backlink is not equal to the token backlink: tx.backlink='%x' token.backlink='%x'",
			attr.Backlink, d.GetBacklink())
	}
	return verifyOwnership(u.Bearer(), predicates, &lockTokenOwnershipProver{tx: tx, attr: attr})
}
