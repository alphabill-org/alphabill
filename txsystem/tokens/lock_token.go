package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *LockTokensModule) handleLockTokenTx() txsystem.GenericExecuteFunc[LockTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *LockTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateLockTokenTx(tx, attr, exeCtx); err != nil {
			return nil, fmt.Errorf("invalid lock token tx: %w", err)
		}
		updateFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				return m.updateLockTokenData(data, tx, attr, exeCtx.CurrentBlockNr)
			},
		)
		if err := m.state.Apply(updateFn); err != nil {
			return nil, fmt.Errorf("failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: m.feeCalculator(), TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func (m *LockTokensModule) updateLockTokenData(data state.UnitData, tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64) (state.UnitData, error) {
	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return updateLockFungibleTokenData(data, tx, attr, roundNumber)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return updateLockNonFungibleTokenData(data, tx, attr, roundNumber)
	} else {
		return nil, fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func updateLockNonFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64) (state.UnitData, error) {
	d, ok := data.(*NonFungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Counter += 1
	d.Locked = attr.LockStatus
	return d, nil
}

func updateLockFungibleTokenData(data state.UnitData, tx *types.TransactionOrder, attr *LockTokenAttributes, roundNumber uint64) (state.UnitData, error) {
	d, ok := data.(*FungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v does not contain fungible token data", tx.UnitID())
	}
	d.T = roundNumber
	d.Counter += 1
	d.Locked = attr.LockStatus
	return d, nil
}

func (m *LockTokensModule) validateLockTokenTx(tx *types.TransactionOrder, attr *LockTokenAttributes, exeCtx *txsystem.TxExecutionContext) error {
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

	if tx.UnitID().HasType(FungibleTokenUnitType) {
		return m.validateFungibleLockToken(tx, attr, u, exeCtx)
	} else if tx.UnitID().HasType(NonFungibleTokenUnitType) {
		return m.validateNonFungibleLockToken(tx, attr, u, exeCtx)
	} else {
		return fmt.Errorf("unit id '%s' is not of fungible nor non-fungible token type", tx.UnitID())
	}
}

func (m *LockTokensModule) validateFungibleLockToken(tx *types.TransactionOrder, attr *LockTokenAttributes, u *state.Unit, exeCtx *txsystem.TxExecutionContext) error {
	d, ok := u.Data().(*FungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not fungible token data", tx.UnitID())
	}
	if err := m.validateTokenLock(attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err := runChainedPredicates[*FungibleTokenTypeData](
		exeCtx,
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
	return nil
}

func (m *LockTokensModule) validateNonFungibleLockToken(tx *types.TransactionOrder, attr *LockTokenAttributes, u *state.Unit, exeCtx *txsystem.TxExecutionContext) error {
	d, ok := u.Data().(*NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not non-fungible token data", tx.UnitID())
	}
	if err := m.validateTokenLock(attr, d); err != nil {
		return err
	}

	if err := m.execPredicate(u.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err := runChainedPredicates[*NonFungibleTokenTypeData](
		exeCtx,
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

func (l *LockTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &LockTokenAttributes{
		LockStatus:                   l.LockStatus,
		Counter:                      l.Counter,
		InvariantPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
}

type tokenData interface {
	GetCounter() uint64
	IsLocked() uint64
}

func (m *LockTokensModule) validateTokenLock(attr *LockTokenAttributes, d tokenData) error {
	// token is not locked
	if d.IsLocked() != 0 {
		return errors.New("token is already locked")
	}
	// the new status is a "locked" one
	if attr.LockStatus == 0 {
		return errors.New("lock status cannot be zero-value")
	}
	// the current transaction follows the previous valid transaction with the token
	if attr.Counter != d.GetCounter() {
		return fmt.Errorf("the transaction counter is not equal to the token counter: tx.counter='%d' token.counter='%d'",
			attr.Counter, d.GetCounter())
	}
	return nil
}
