package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/api/predicates"
	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/state"
	"github.com/alphabill-org/alphabill/validator/pkg/tree/avl"
)

func handleUnlockTokenTx(options *Options) txsystem.GenericExecuteFunc[UnlockTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UnlockTokenAttributes, roundNumber uint64) (*types.ServerMetadata, error) {
		if err := validateUnlockTokenTx(tx, attr, options); err != nil {
			return nil, fmt.Errorf("invalid unlock token tx: %w", err)
		}
		// update lock status, round number and backlink
		updateFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				return updateUnlockTokenData(data, tx, roundNumber, options)
			})
		if err := options.state.Apply(updateFn); err != nil {
			return nil, fmt.Errorf("failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: options.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func updateUnlockTokenData(data state.UnitData, tx *types.TransactionOrder, roundNumber uint64, options *Options) (state.UnitData, error) {
	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return updateUnlockFungibleTokenData(data, tx, roundNumber, options)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return updateUnlockNonFungibleTokenData(data, tx, roundNumber, options)
	} else {
		return nil, fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func updateUnlockNonFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, roundNumber uint64, options *Options) (state.UnitData, error) {
	d, ok := data.(*nonFungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.t = roundNumber
	d.backlink = tx.Hash(options.hashAlgorithm)
	d.locked = 0
	return d, nil
}

func updateUnlockFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, roundNumber uint64, options *Options) (state.UnitData, error) {
	d, ok := data.(*fungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.t = roundNumber
	d.backlink = tx.Hash(options.hashAlgorithm)
	d.locked = 0
	return d, nil
}

func validateUnlockTokenTx(tx *types.TransactionOrder, attr *UnlockTokenAttributes, options *Options) error {
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
			return fmt.Errorf("unit %v does not exist: %w", tx.UnitID(), err)
		}
		return err
	}

	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return validateUnlockFungibleToken(tx, attr, options, u)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return validateUnlockNonFungibleToken(tx, attr, options, u)
	} else {
		return fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func validateUnlockNonFungibleToken(tx *types.TransactionOrder, attr *UnlockTokenAttributes, options *Options, u *state.Unit) error {
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
	return validateUnlockToken(u, tx, attr, predicates, d)
}

func validateUnlockFungibleToken(tx *types.TransactionOrder, attr *UnlockTokenAttributes, options *Options, u *state.Unit) error {
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
	return validateUnlockToken(u, tx, attr, predicates, d)
}

type unlockTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *UnlockTokenAttributes
}

func (t *unlockTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *unlockTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *unlockTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
}

func (l *UnlockTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &UnlockTokenAttributes{
		Backlink:                     l.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}

func validateUnlockToken(u *state.Unit, tx *types.TransactionOrder, attr *UnlockTokenAttributes, predicates []predicates.PredicateBytes, d tokenData) error {
	// the token is locked
	if d.Locked() == 0 {
		return errors.New("token is already unlocked")
	}
	// the current transaction follows the previous valid transaction with the token
	if !bytes.Equal(attr.Backlink, d.Backlink()) {
		return fmt.Errorf("the transaction backlink is not equal to the token backlink: tx.backlink='%x' token.backlink='%x'",
			attr.Backlink, d.Backlink())
	}
	return verifyOwnership(u.Bearer(), predicates, &unlockTokenOwnershipProver{tx: tx, attr: attr})
}
