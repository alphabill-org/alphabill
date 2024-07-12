package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *LockTokensModule) updateLockTokenData(data types.UnitData, tx *types.TransactionOrder, attr *tokens.LockTokenAttributes, roundNumber uint64) (types.UnitData, error) {
	if tx.UnitID().HasType(tokens.FungibleTokenUnitType) {
		return updateLockFungibleTokenData(data, tx, attr, roundNumber)
	} else if tx.UnitID().HasType(tokens.NonFungibleTokenUnitType) {
		return updateLockNonFungibleTokenData(data, tx, attr, roundNumber)
	} else {
		return nil, fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func updateLockNonFungibleTokenData(data types.UnitData, tx *types.TransactionOrder, attr *tokens.LockTokenAttributes, roundNumber uint64) (types.UnitData, error) {
	d, ok := data.(*tokens.NonFungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Counter += 1
	d.Locked = attr.LockStatus
	return d, nil
}

func updateLockFungibleTokenData(data types.UnitData, tx *types.TransactionOrder, attr *tokens.LockTokenAttributes, roundNumber uint64) (types.UnitData, error) {
	d, ok := data.(*tokens.FungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Counter += 1
	d.Locked = attr.LockStatus
	return d, nil
}

func (m *LockTokensModule) executeLockTokensTx(tx *types.TransactionOrder, attr *tokens.LockTokenAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	updateFn := state.UpdateUnitData(tx.UnitID(),
		func(data types.UnitData) (types.UnitData, error) {
			return m.updateLockTokenData(data, tx, attr, exeCtx.CurrentRound())
		},
	)
	if err := m.state.Apply(updateFn); err != nil {
		return nil, fmt.Errorf("failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{tx.UnitID()}}, nil
}

func (m *LockTokensModule) validateLockTokenTx(tx *types.TransactionOrder, attr *tokens.LockTokenAttributes, exeCtx txtypes.ExecutionContext) error {
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
			return fmt.Errorf("unit '%v' does not exist: %w", tx.UnitID(), err)
		}
		return err
	}

	if tx.UnitID().HasType(tokens.FungibleTokenUnitType) {
		return m.validateFungibleLockToken(tx, attr, u, exeCtx)
	} else if tx.UnitID().HasType(tokens.NonFungibleTokenUnitType) {
		return m.validateNonFungibleLockToken(tx, attr, u, exeCtx)
	} else {
		return fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func (m *LockTokensModule) validateFungibleLockToken(tx *types.TransactionOrder, attr *tokens.LockTokenAttributes, u *state.Unit, exeCtx txtypes.ExecutionContext) error {
	d, ok := u.Data().(*tokens.FungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not fungible token data", tx.UnitID())
	}
	if err := m.validateTokenLock(attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err := runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
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
	return nil
}

func (m *LockTokensModule) validateNonFungibleLockToken(tx *types.TransactionOrder, attr *tokens.LockTokenAttributes, u *state.Unit, exeCtx txtypes.ExecutionContext) error {
	d, ok := u.Data().(*tokens.NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not non-fungible token data", tx.UnitID())
	}
	if err := m.validateTokenLock(attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		exeCtx,
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

type unitData interface {
	GetCounter() uint64
	IsLocked() uint64
}

func (m *LockTokensModule) validateTokenLock(attr *tokens.LockTokenAttributes, tokenData unitData) error {
	// token is not locked
	if tokenData.IsLocked() != 0 {
		return errors.New("token is already locked")
	}
	// the new status is a "locked" one
	if attr.LockStatus == 0 {
		return errors.New("lock status cannot be zero-value")
	}
	// the current transaction follows the previous valid transaction with the token
	if attr.Counter != tokenData.GetCounter() {
		return fmt.Errorf("the transaction counter is not equal to the token counter: tx.counter='%d' token.counter='%d'",
			attr.Counter, tokenData.GetCounter())
	}
	return nil
}
