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

func handleJoinFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[JoinFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Join Fungible Token tx: %v", tx)
		sum, err := validateJoinFungibleToken(tx, attr, options)
		if err != nil {
			return nil, fmt.Errorf("invalid join fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.UpdateData(unitID,
				func(data rma.UnitData) rma.UnitData {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						// No change in case of incorrect data type.
						return data
					}
					return &fungibleTokenData{
						tokenType: d.tokenType,
						value:     sum,
						t:         currentBlockNr,
						backlink:  h,
					}
				}, h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateJoinFungibleToken(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, options *Options) (uint64, error) {
	bearer, d, err := getFungibleTokenData(util.BytesToUint256(tx.UnitID()), options.state)
	if err != nil {
		return 0, err
	}
	transactions := attr.BurnTransactions
	proofs := attr.Proofs
	if len(transactions) != len(proofs) {
		return 0, fmt.Errorf("invalid count of proofs: expected %v, got %v", len(transactions), len(proofs))
	}
	sum := d.value
	for i, btx := range transactions {
		prevSum := sum
		btxAttr := &BurnFungibleTokenAttributes{}
		if err := btx.TransactionOrder.UnmarshalAttributes(btxAttr); err != nil {
			return 0, fmt.Errorf("failed to unmarshal burn fungible token attributes")
		}

		sum += btxAttr.Value
		if prevSum > sum { // overflow
			return 0, errors.New("invalid sum of tokens: uint64 overflow")
		}
		tokenTypeID := util.Uint256ToBytes(d.tokenType)
		if !bytes.Equal(btxAttr.TypeID, tokenTypeID) {
			return 0, fmt.Errorf("the type of the burned source token does not match the type of target token: expected %X, got %X", tokenTypeID, btxAttr.TypeID)
		}

		if !bytes.Equal(btxAttr.Nonce, attr.Backlink) {
			return 0, fmt.Errorf("the source tokens weren't burned to join them to the target token: source %X, target %X", btxAttr.Nonce, attr.Backlink)
		}
		proof := proofs[i]

		if err = types.VerifyTxProof(proof, btx, options.trustBase, options.hashAlgorithm); err != nil {
			return 0, fmt.Errorf("proof is not valid: %w", err)
		}
	}
	if !bytes.Equal(d.backlink, attr.Backlink) {
		return 0, fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		options.state,
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return 0, err
	}
	return sum, verifyOwnership(bearer, predicates, &joinFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

type joinFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *JoinFungibleTokenAttributes
}

func (t *joinFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *joinFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *joinFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	if len(t.attr.InvariantPredicateSignatures) == 0 {
		return t.tx.PayloadBytes()
	}
	// exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &JoinFungibleTokenAttributes{
		BurnTransactions:             t.attr.BurnTransactions,
		Proofs:                       t.attr.Proofs,
		Backlink:                     t.attr.Backlink,
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

func (j *JoinFungibleTokenAttributes) GetBurnTransactions() []*types.TransactionRecord {
	return j.BurnTransactions
}

func (j *JoinFungibleTokenAttributes) SetBurnTransactions(burnTransactions []*types.TransactionRecord) {
	j.BurnTransactions = burnTransactions
}

func (j *JoinFungibleTokenAttributes) GetProofs() []*types.TxProof {
	return j.Proofs
}

func (j *JoinFungibleTokenAttributes) SetProofs(proofs []*types.TxProof) {
	j.Proofs = proofs
}

func (j *JoinFungibleTokenAttributes) GetBacklink() []byte {
	return j.Backlink
}

func (j *JoinFungibleTokenAttributes) SetBacklink(backlink []byte) {
	j.Backlink = backlink
}

func (j *JoinFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return j.InvariantPredicateSignatures
}

func (j *JoinFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	j.InvariantPredicateSignatures = signatures
}

func (j *JoinFungibleTokenAttributes) MarshalCBOR() ([]byte, error) {
	return cbor.Marshal(j)
}

func (j *JoinFungibleTokenAttributes) UnmarshalCBOR(data []byte) error {
	return cbor.Unmarshal(data, j)
}
