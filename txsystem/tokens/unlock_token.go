package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *LockTokensModule) handleUnlockTokenTx() txsystem.GenericExecuteFunc[UnlockTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UnlockTokenAttributes, roundNumber uint64) (*types.ServerMetadata, error) {
		if err := m.validateUnlockTokenTx(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid unlock token tx: %w", err)
		}
		// update lock status, round number and backlink
		updateFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				return m.updateUnlockTokenData(data, tx, roundNumber)
			})
		if err := m.state.Apply(updateFn); err != nil {
			return nil, fmt.Errorf("failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: m.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func (m *LockTokensModule) updateUnlockTokenData(data state.UnitData, tx *types.TransactionOrder, roundNumber uint64) (state.UnitData, error) {
	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return updateUnlockFungibleTokenData(data, tx, roundNumber, m.hashAlgorithm)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return updateUnlockNonFungibleTokenData(data, tx, roundNumber, m.hashAlgorithm)
	} else {
		return nil, fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func updateUnlockNonFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, roundNumber uint64, hashAlgorithm crypto.Hash) (state.UnitData, error) {
	d, ok := data.(*NonFungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Backlink = tx.Hash(hashAlgorithm)
	d.Locked = 0
	return d, nil
}

func updateUnlockFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, roundNumber uint64, hashAlgorithm crypto.Hash) (state.UnitData, error) {
	d, ok := data.(*FungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Backlink = tx.Hash(hashAlgorithm)
	d.Locked = 0
	return d, nil
}

func (m *LockTokensModule) validateUnlockTokenTx(tx *types.TransactionOrder, attr *UnlockTokenAttributes) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if attr == nil {
		return errors.New("attributes is nil")
	}

	// unit id identifies an existing fungible or non-fungible token
	u, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return fmt.Errorf("unit %v does not exist: %w", tx.UnitID(), err)
		}
		return err
	}

	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return m.validateUnlockFungibleToken(tx, attr, u)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return m.validateUnlockNonFungibleToken(tx, attr, u)
	} else {
		return fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func (m *LockTokensModule) validateUnlockNonFungibleToken(tx *types.TransactionOrder, attr *UnlockTokenAttributes, u *state.Unit) error {
	d, ok := u.Data().(*NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not non-fungible token data", tx.UnitID())
	}
	if err := validateUnlockToken(u, tx, attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("bearer predicate: %w", err)
	}
	err := runChainedPredicates[*NonFungibleTokenTypeData](
		tx,
		d.NftType,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return nil
}

func (m *LockTokensModule) validateUnlockFungibleToken(tx *types.TransactionOrder, attr *UnlockTokenAttributes, u *state.Unit) error {
	d, ok := u.Data().(*FungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not fungible token data", tx.UnitID())
	}
	if err := validateUnlockToken(u, tx, attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("bearer predicate: %w", err)
	}
	err := runChainedPredicates[*FungibleTokenTypeData](
		tx,
		d.TokenType,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return validateUnlockToken(u, tx, attr, d)
}

func (l *UnlockTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &UnlockTokenAttributes{
		Backlink:                     l.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}

func validateUnlockToken(u *state.Unit, tx *types.TransactionOrder, attr *UnlockTokenAttributes, d tokenData) error {
	// the token is locked
	if d.IsLocked() == 0 {
		return errors.New("token is already unlocked")
	}
	// the current transaction follows the previous valid transaction with the token
	if !bytes.Equal(attr.Backlink, d.GetBacklink()) {
		return fmt.Errorf("the transaction backlink is not equal to the token backlink: tx.backlink='%x' token.backlink='%x'",
			attr.Backlink, d.GetBacklink())
	}
	return nil
}
