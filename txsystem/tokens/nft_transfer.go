package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (n *NonFungibleTokensModule) executeTransferNFT(tx *types.TransactionOrder, attr *tokens.TransferNonFungibleTokenAttributes, _ *tokens.TransferNonFungibleTokenAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()

	// 1. N[T.ι].D.φ ← T.A.φ
	// 2. N[T.ι].D.c ← N[T.ι].D.c + 1
	if err := n.state.Apply(
		state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
			d, ok := data.(*tokens.NonFungibleTokenData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
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

func (n *NonFungibleTokensModule) validateTransferNFT(tx *types.TransactionOrder, attr *tokens.TransferNonFungibleTokenAttributes, authProof *tokens.TransferNonFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	unitID := tx.GetUnitID()
	if !unitID.HasType(tokens.NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := n.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*tokens.NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("validate nft transfer: unit %v is not a non-fungible token type", unitID)
	}
	if data.Locked != 0 {
		return errors.New("token is locked")
	}
	if data.Counter != attr.Counter {
		return fmt.Errorf("invalid counter: expected %d, got %d", data.Counter, attr.Counter)
	}
	tokenTypeID := data.TypeID
	if !bytes.Equal(attr.TypeID, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenTypeID, attr.TypeID)
	}

	if err = n.execPredicate(data.OwnerPredicate, authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("evaluating owner predicate: %w", err)
	}
	err = runChainedPredicates[*tokens.NonFungibleTokenTypeData](
		exeCtx,
		tx.AuthProofSigBytes,
		tokenTypeID,
		authProof.TokenTypeOwnerProofs,
		n.execPredicate,
		func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.TokenTypeOwnerPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("token type owner predicate: %w", err)
	}
	return nil
}
