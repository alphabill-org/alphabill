package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
)

func handleSplitFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[SplitFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Split Fungible Token tx: %v", tx)
		if err := validateSplitFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid split fungible token tx: %w", err)
		}
		unitID := tx.UnitID()
		u, err := options.state.GetUnit(unitID, false)
		if err != nil {
			return nil, err
		}
		d := u.Data().(*fungibleTokenData)
		// add new token unit
		newTokenID := txutil.SameShardID(unitID, HashForIDCalculation(tx, options.hashAlgorithm))
		logger.Debug("Adding a fungible token with ID %v", newTokenID)

		fee := options.feeCalculator()
		sm := &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID, newTokenID}}
		txr := types.TransactionRecord{TransactionOrder: tx, ServerMetadata: sm}
		txHash := txr.Hash(options.hashAlgorithm)

		// update state
		if err := options.state.Apply(
			state.AddUnit(newTokenID,
				attr.NewBearer,
				&fungibleTokenData{
					tokenType: d.tokenType,
					value:     attr.TargetValue,
					t:         currentBlockNr,
					backlink:  txHash,
				}),
			state.UpdateUnitData(unitID,
				func(data state.UnitData) (newData state.UnitData) {
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
				})); err != nil {
			return nil, err
		}

		return sm, nil
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

func validateSplitFungibleToken(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), s, hashAlgorithm)
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
	if !bytes.Equal(attr.TypeID, d.tokenType) {
		return fmt.Errorf("invalid type identifier: expected '%X', got '%X'", d.tokenType, attr.TypeID)
	}

	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		hashAlgorithm,
		s,
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) types.UnitID {
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
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
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

func (s *SplitFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &SplitFungibleTokenAttributes{
		NewBearer:                    s.NewBearer,
		TargetValue:                  s.TargetValue,
		Nonce:                        s.Nonce,
		Backlink:                     s.Backlink,
		TypeID:                       s.TypeID,
		RemainingValue:               s.RemainingValue,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
