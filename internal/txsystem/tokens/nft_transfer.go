package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
)

func handleTransferNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[TransferNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Transfer Non-Fungible Token tx: %v", tx)
		if err := validateTransferNonFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid transfer none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.SetOwner(unitID, attr.NewBearer, h),
			rma.UpdateData(unitID, func(data rma.UnitData) (newData rma.UnitData) {
				d, ok := data.(*nonFungibleTokenData)
				if !ok {
					return data
				}
				d.t = currentBlockNr
				d.backlink = tx.Hash(options.hashAlgorithm)
				return data
			}, h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateTransferNonFungibleToken(tx *types.TransactionOrder, attr *TransferNonFungibleTokenAttributes, state *rma.Tree) error {
	unitID := util.BytesToUint256(tx.UnitID())
	u, err := state.GetUnit(unitID)
	if err != nil {
		return err
	}
	data, ok := u.Data.(*nonFungibleTokenData)
	if !ok {
		return fmt.Errorf("validate nft transfer: unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, attr.Backlink) {
		return errors.New("validate nft transfer: invalid backlink")
	}
	tokenTypeID := util.Uint256ToBytes(data.nftType)
	if !bytes.Equal(attr.NFTTypeID, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%X', got '%X'", tokenTypeID, attr.NFTTypeID)
	}

	// signature given in the transaction request satisfies the predicate obtained by concatenating all the token
	// invariant clauses along the type inheritance chain.
	predicates, err := getChainedPredicates(
		state,
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyOwnership(Predicate(u.Bearer), predicates, &transferNFTTokenOwnershipProver{tx: tx, attr: attr})
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

func (t *TransferNonFungibleTokenAttributes) GetNFTTypeID() []byte {
	return t.NFTTypeID
}

func (t *TransferNonFungibleTokenAttributes) SetNFTTypeID(nftTypeID []byte) {
	t.NFTTypeID = nftTypeID
}

func (t *TransferNonFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return t.InvariantPredicateSignatures
}

func (t *TransferNonFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	t.InvariantPredicateSignatures = signatures
}
