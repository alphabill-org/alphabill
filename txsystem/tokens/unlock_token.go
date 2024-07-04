package tokens

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *LockTokensModule) updateUnlockTokenData(data types.UnitData, tx *types.TransactionOrder, roundNumber uint64) (types.UnitData, error) {
	if tx.UnitID().HasType(tokens.FungibleTokenUnitType) {
		return updateUnlockFungibleTokenData(data, tx, roundNumber)
	} else if tx.UnitID().HasType(tokens.NonFungibleTokenUnitType) {
		return updateUnlockNonFungibleTokenData(data, tx, roundNumber, m.hashAlgorithm)
	} else {
		return nil, fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func updateUnlockNonFungibleTokenData(data types.UnitData, tx *types.TransactionOrder, roundNumber uint64, hashAlgorithm crypto.Hash) (types.UnitData, error) {
	d, ok := data.(*tokens.NonFungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Counter += 1
	d.Locked = 0
	return d, nil
}

func updateUnlockFungibleTokenData(data types.UnitData, tx *types.TransactionOrder, roundNumber uint64) (types.UnitData, error) {
	d, ok := data.(*tokens.FungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Counter += 1
	d.Locked = 0
	return d, nil
}

func (m *LockTokensModule) executeUnlockTokenTx(tx *types.TransactionOrder, _ *tokens.UnlockTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
	// update lock status, round number and counter
	updateFn := state.UpdateUnitData(tx.UnitID(),
		func(data types.UnitData) (types.UnitData, error) {
			return m.updateUnlockTokenData(data, tx, exeCtx.CurrentBlockNumber)
		})
	if err := m.state.Apply(updateFn); err != nil {
		return nil, fmt.Errorf("failed to update state: %w", err)
	}
	return &types.ServerMetadata{ActualFee: m.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
}

func (m *LockTokensModule) validateUnlockTokenTx(tx *types.TransactionOrder, attr *tokens.UnlockTokenAttributes, _ *txsystem.TxExecutionContext) error {
	// unit id identifies an existing fungible or non-fungible token
	u, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return fmt.Errorf("unit %v does not exist: %w", tx.UnitID(), err)
		}
		return err
	}

	if tx.UnitID().HasType(tokens.FungibleTokenUnitType) {
		return m.validateUnlockFungibleToken(tx, attr, u)
	} else if tx.UnitID().HasType(tokens.NonFungibleTokenUnitType) {
		return m.validateUnlockNonFungibleToken(tx, attr, u)
	} else {
		return fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func (m *LockTokensModule) validateUnlockNonFungibleToken(tx *types.TransactionOrder, attr *tokens.UnlockTokenAttributes, u *state.Unit) error {
	d, ok := u.Data().(*tokens.NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not non-fungible token data", tx.UnitID())
	}
	if err := validateUnlockToken(attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("bearer predicate: %w", err)
	}
	err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		tx,
		d.TypeID,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return nil
}

func (m *LockTokensModule) validateUnlockFungibleToken(tx *types.TransactionOrder, attr *tokens.UnlockTokenAttributes, u *state.Unit) error {
	d, ok := u.Data().(*tokens.FungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not fungible token data", tx.UnitID())
	}
	if err := validateUnlockToken(attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("bearer predicate: %w", err)
	}
	err := runChainedPredicates[*tokens.FungibleTokenTypeData](
		tx,
		d.TokenType,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return validateUnlockToken(attr, d)
}

func validateUnlockToken(attr *tokens.UnlockTokenAttributes, d unitData) error {
	// the token is locked
	if d.IsLocked() == 0 {
		return errors.New("token is already unlocked")
	}
	// the current transaction follows the previous valid transaction with the token
	if attr.Counter != d.GetCounter() {
		return fmt.Errorf("the transaction counter is not equal to the token counter: tx.counter='%d' token.counter='%d'",
			attr.Counter, d.GetCounter())
	}
	return nil
}
