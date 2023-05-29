package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
)

func handleSplitFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[SplitFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Split Fungible Token tx: %v", tx)
		if err := validateSplitFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid split fungible token tx: %w", err)
		}
		unitID := util.BytesToUint256(tx.UnitID())
		u, err := options.state.GetUnit(unitID)
		if err != nil {
			return nil, err
		}
		d := u.Data.(*fungibleTokenData)
		// add new token unit
		newTokenID := txutil.SameShardID(unitID, HashForIDCalculation(tx, options.hashAlgorithm))
		logger.Debug("Adding a fungible token with ID %v", newTokenID)

		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		txHash := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, txHash),
			rma.AddItem(newTokenID,
				attr.NewBearer,
				&fungibleTokenData{
					tokenType: d.tokenType,
					value:     attr.TargetValue,
					t:         currentBlockNr,
					backlink:  txHash,
				}, txHash),
			rma.UpdateData(unitID,
				func(data rma.UnitData) (newData rma.UnitData) {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						// No change in case of incorrect data type.
						return data
					}
					return &fungibleTokenData{
						tokenType: d.tokenType,
						value:     d.value - attr.TargetValue,
						t:         currentBlockNr,
						backlink:  txHash,
					}
				}, txHash)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func HashForIDCalculation(tx *types.TransactionOrder, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	idBytes := util.BytesToUint256(tx.UnitID()).Bytes32()
	hasher.Write(idBytes[:])
	hasher.Write(tx.Payload.Attributes)
	hasher.Write(util.Uint64ToBytes(tx.Timeout()))
	return hasher.Sum(nil)
}

func validateSplitFungibleToken(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes, state *rma.Tree) error {
	bearer, d, err := getFungibleTokenData(util.BytesToUint256(tx.UnitID()), state)
	if err != nil {
		return err
	}
	if attr.TargetValue == 0 {
		return errors.New("when splitting a token the value assigned to the new token must be greater than zero")
	}
	if attr.RemainingValue == 0 {
		return errors.New("when splitting a token the remaining value of the token must be greater than zero")
	}

	if d.value < attr.TargetValue {
		return fmt.Errorf("invalid token value: max allowed %v, got %v", d.value, attr.TargetValue)
	}
	if rm := d.value - attr.TargetValue; attr.RemainingValue != rm {
		return errors.New("remaining value must equal to the original value minus target value")
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
	return verifyOwnership(bearer, predicates, &splitFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

type splitFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *SplitFungibleTokenAttributes
}

func (t *splitFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *splitFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *splitFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	if len(t.attr.InvariantPredicateSignatures) == 0 {
		return t.tx.PayloadBytes()
	}
	// exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &SplitFungibleTokenAttributes{
		NewBearer:                    t.attr.NewBearer,
		TargetValue:                  t.attr.TargetValue,
		Nonce:                        t.attr.Nonce,
		Backlink:                     t.attr.Backlink,
		TypeID:                       t.attr.TypeID,
		RemainingValue:               t.attr.RemainingValue,
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

func (s *SplitFungibleTokenAttributes) GetNewBearer() []byte {
	return s.NewBearer
}

func (s *SplitFungibleTokenAttributes) SetNewBearer(newBearer []byte) {
	s.NewBearer = newBearer
}

func (s *SplitFungibleTokenAttributes) GetTargetValue() uint64 {
	return s.TargetValue
}

func (s *SplitFungibleTokenAttributes) SetTargetValue(targetValue uint64) {
	s.TargetValue = targetValue
}

func (s *SplitFungibleTokenAttributes) GetNonce() []byte {
	return s.Nonce
}

func (s *SplitFungibleTokenAttributes) SetNonce(nonce []byte) {
	s.Nonce = nonce
}

func (s *SplitFungibleTokenAttributes) GetBacklink() []byte {
	return s.Backlink
}

func (s *SplitFungibleTokenAttributes) SetBacklink(backlink []byte) {
	s.Backlink = backlink
}

func (s *SplitFungibleTokenAttributes) GetTypeID() []byte {
	return s.TypeID
}

func (s *SplitFungibleTokenAttributes) SetTypeID(typeID []byte) {
	s.TypeID = typeID
}

func (s *SplitFungibleTokenAttributes) GetRemainingValue() uint64 {
	return s.RemainingValue
}

func (s *SplitFungibleTokenAttributes) SetRemainingValue(remainingValue uint64) {
	s.RemainingValue = remainingValue
}

func (s *SplitFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return s.InvariantPredicateSignatures
}

func (s *SplitFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	s.InvariantPredicateSignatures = signatures
}
