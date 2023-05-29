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

func handleTransferFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[TransferFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Transfer Fungible Token tx: %v", tx)
		if err := validateTransferFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid transfer fungible token tx: %w", err)
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
			rma.UpdateData(unitID,
				func(data rma.UnitData) (newData rma.UnitData) {
					d, ok := data.(*fungibleTokenData)
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

func validateTransferFungibleToken(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes, state *rma.Tree) error {
	bearer, d, err := getFungibleTokenData(util.BytesToUint256(tx.UnitID()), state)
	if err != nil {
		return err
	}
	if d.value != attr.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.value, attr.Value)
	}

	if !bytes.Equal(d.backlink, attr.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
	}

	tokenTypeID := util.Uint256ToBytes(d.tokenType)
	if !bytes.Equal(attr.TypeID, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%X', got '%X'", tokenTypeID, attr.TypeID)
	}

	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		state,
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyOwnership(bearer, predicates, &transferFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

func getFungibleTokenData(unitID *uint256.Int, state *rma.Tree) (Predicate, *fungibleTokenData, error) {
	if unitID.IsZero() {
		return nil, nil, errors.New(ErrStrUnitIDIsZero)
	}
	u, err := state.GetUnit(unitID)
	if err != nil {
		if errors.Is(err, rma.ErrUnitNotFound) {
			return nil, nil, fmt.Errorf("unit %v does not exist: %w", unitID, err)
		}
		return nil, nil, err
	}
	d, ok := u.Data.(*fungibleTokenData)
	if !ok {
		return nil, nil, fmt.Errorf("unit %v is not fungible token data", unitID)
	}
	return Predicate(u.Bearer), d, nil
}

type transferFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *TransferFungibleTokenAttributes
}

func (t *transferFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *transferFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *transferFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	if len(t.attr.InvariantPredicateSignatures) == 0 {
		return t.tx.PayloadBytes()
	}
	// exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &TransferFungibleTokenAttributes{
		NewBearer:                    t.attr.NewBearer,
		Value:                        t.attr.Value,
		Nonce:                        t.attr.Nonce,
		Backlink:                     t.attr.Backlink,
		TypeID:                       t.attr.TypeID,
		InvariantPredicateSignatures: nil,
	}
	attrBytes, err := cbor.Marshal(signatureAttr)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attributes: %w", err)
	}
	payload := &types.Payload{
		SystemID:       t.tx.Payload.SystemID,
		Type:           t.tx.Payload.Type,
		UnitID:         t.tx.Payload.UnitID,
		Attributes:     attrBytes,
		ClientMetadata: t.tx.Payload.ClientMetadata,
	}
	return payload.Bytes()
}

func (t *TransferFungibleTokenAttributes) GetNewBearer() []byte {
	return t.NewBearer
}

func (t *TransferFungibleTokenAttributes) SetNewBearer(newBearer []byte) {
	t.NewBearer = newBearer
}

func (t *TransferFungibleTokenAttributes) GetValue() uint64 {
	return t.Value
}

func (t *TransferFungibleTokenAttributes) SetValue(value uint64) {
	t.Value = value
}

func (t *TransferFungibleTokenAttributes) GetNonce() []byte {
	return t.Nonce
}

func (t *TransferFungibleTokenAttributes) SetNonce(nonce []byte) {
	t.Nonce = nonce
}

func (t *TransferFungibleTokenAttributes) GetBacklink() []byte {
	return t.Backlink
}

func (t *TransferFungibleTokenAttributes) SetBacklink(backlink []byte) {
	t.Backlink = backlink
}

func (t *TransferFungibleTokenAttributes) GetTypeID() []byte {
	return t.TypeID
}

func (t *TransferFungibleTokenAttributes) SetTypeID(typeID []byte) {
	t.TypeID = typeID
}

func (t *TransferFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return t.InvariantPredicateSignatures
}

func (t *TransferFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	t.InvariantPredicateSignatures = signatures
}
