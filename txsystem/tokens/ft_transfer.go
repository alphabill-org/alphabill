package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *FungibleTokensModule) executeTransferFT(tx *types.TransactionOrder, attr *tokens.TransferFungibleTokenAttributes, _ *tokens.TransferFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()

	// 1. N[T.ι].D.φ ← T.A.φ
	// 2. N[T.ι].D.c ← N[T.ι].D.c + 1
	if err := m.state.Apply(
		state.UpdateUnitData(unitID,
			func(data types.UnitData) (types.UnitData, error) {
				d, ok := data.(*tokens.FungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
				}
				d.OwnerPredicate = attr.NewOwnerPredicate
				d.Counter += 1
				return d, nil
			}),
	); err != nil {
		return nil, err
	}

	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateTransferFT(tx *types.TransactionOrder, attr *tokens.TransferFungibleTokenAttributes, authProof *tokens.TransferFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	tokenData, err := getFungibleTokenData(tx.UnitID, m.state)
	if err != nil {
		return err
	}
	if tokenData.Locked != 0 {
		return fmt.Errorf("token is locked")
	}
	if tokenData.Value != attr.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", tokenData.Value, attr.Value)
	}
	if tokenData.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", tokenData.Counter, attr.Counter)
	}
	if !bytes.Equal(attr.TypeID, tokenData.TypeID) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenData.TypeID, attr.TypeID)
	}
	if err = m.execPredicate(tokenData.OwnerPredicate, authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		exeCtx,
		tx.AuthProofSigBytes,
		tokenData.TypeID,
		authProof.TokenTypeOwnerProofs,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.TokenTypeOwnerPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type owner predicate: %w", err)
	}
	return nil
}

func getFungibleTokenData(unitID types.UnitID, s *state.State) (*tokens.FungibleTokenData, error) {
	if !unitID.HasType(tokens.FungibleTokenUnitType) {
		return nil, errors.New(ErrStrInvalidUnitID)
	}

	u, err := s.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return nil, fmt.Errorf("unit %v does not exist: %w", unitID, err)
		}
		return nil, err
	}
	d, ok := u.Data().(*tokens.FungibleTokenData)
	if !ok {
		return nil, fmt.Errorf("unit %v is not fungible token data", unitID)
	}
	return d, nil
}
