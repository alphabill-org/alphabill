package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

func (n *NonFungibleTokensModule) handleTransferNonFungibleTokenTx() txsystem.GenericExecuteFunc[TransferNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferNonFungibleTokenAttributes, ctx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := n.validateTransferNonFungibleToken(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid transfer non-fungible token tx: %w", err)
		}
		fee := n.feeCalculator()

		unitID := tx.UnitID()

		// update state
		if err := n.state.Apply(
			state.SetOwner(unitID, attr.NewBearer),
			state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
				d, ok := data.(*NonFungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
				}
				d.T = ctx.CurrentBlockNr
				d.Backlink = tx.Hash(n.hashAlgorithm)
				return d, nil
			})); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (n *NonFungibleTokensModule) validateTransferNonFungibleToken(tx *types.TransactionOrder, attr *TransferNonFungibleTokenAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := n.state.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("validate nft transfer: unit %v is not a non-fungible token type", unitID)
	}
	if data.Locked != 0 {
		return errors.New("token is locked")
	}
	if !bytes.Equal(data.Backlink, attr.Backlink) {
		return errors.New("validate nft transfer: invalid backlink")
	}
	tokenTypeID := data.NftType
	if !bytes.Equal(attr.NFTTypeID, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenTypeID, attr.NFTTypeID)
	}

	if err = n.execPredicate(u.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("executing bearer predicate: %w", err)
	}
	err = runChainedPredicates[*NonFungibleTokenTypeData](
		tx,
		data.NftType,
		attr.InvariantPredicateSignatures,
		n.execPredicate,
		func(d *NonFungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.InvariantPredicate
		},
		n.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf(`token type "InvariantPredicate": %w`, err)
	}
	return nil
}

func (t *TransferNonFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude SubTypeCreationPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &TransferNonFungibleTokenAttributes{
		NewBearer:                    t.NewBearer,
		Nonce:                        t.Nonce,
		Backlink:                     t.Backlink,
		NFTTypeID:                    t.NFTTypeID,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}

func (t *TransferNonFungibleTokenAttributes) GetNewBearer() []byte {
	return t.NewBearer
}

func (t *TransferNonFungibleTokenAttributes) SetNewBearer(newBearer []byte) {
	t.NewBearer = newBearer
}

func (t *TransferNonFungibleTokenAttributes) GetNonce() []byte {
	return t.Nonce
}

func (t *TransferNonFungibleTokenAttributes) SetNonce(nonce []byte) {
	t.Nonce = nonce
}

func (t *TransferNonFungibleTokenAttributes) GetBacklink() []byte {
	return t.Backlink
}

func (t *TransferNonFungibleTokenAttributes) SetBacklink(backlink []byte) {
	t.Backlink = backlink
}

func (t *TransferNonFungibleTokenAttributes) GetNFTTypeID() types.UnitID {
	return t.NFTTypeID
}

func (t *TransferNonFungibleTokenAttributes) SetNFTTypeID(nftTypeID types.UnitID) {
	t.NFTTypeID = nftTypeID
}

func (t *TransferNonFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return t.InvariantPredicateSignatures
}

func (t *TransferNonFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	t.InvariantPredicateSignatures = signatures
}
