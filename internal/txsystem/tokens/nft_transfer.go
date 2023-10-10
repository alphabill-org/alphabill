package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
)

func handleTransferNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[TransferNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := validateTransferNonFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid transfer non-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		unitID := tx.UnitID()

		// update state
		if err := options.state.Apply(
			state.SetOwner(unitID, attr.NewBearer),
			state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
				d, ok := data.(*nonFungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
				}
				d.t = currentBlockNr
				d.backlink = tx.Hash(options.hashAlgorithm)
				return d, nil
			})); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateTransferNonFungibleToken(tx *types.TransactionOrder, attr *TransferNonFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	unitID := tx.UnitID()
	if !unitID.HasType(NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := s.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*nonFungibleTokenData)
	if !ok {
		return fmt.Errorf("validate nft transfer: unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, attr.Backlink) {
		return errors.New("validate nft transfer: invalid backlink")
	}
	tokenTypeID := data.nftType
	if !bytes.Equal(attr.NFTTypeID, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", tokenTypeID, attr.NFTTypeID)
	}

	// signature given in the transaction request satisfies the predicate obtained by concatenating all the token
	// invariant clauses along the type inheritance chain.
	predicates, err := getChainedPredicates(
		hashAlgorithm,
		s,
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *nonFungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyOwnership(u.Bearer(), predicates, &transferNFTTokenOwnershipProver{tx: tx, attr: attr})
}

type transferNFTTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *TransferNonFungibleTokenAttributes
}

func (t *transferNFTTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *transferNFTTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *transferNFTTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
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
