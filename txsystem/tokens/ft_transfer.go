package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/validator/internal/predicates"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/alphabill-org/alphabill/validator/pkg/tree/avl"
)

func handleTransferFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[TransferFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := validateTransferFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid transfer fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		unitID := tx.UnitID()

		// update state
		if err := options.state.Apply(
			state.SetOwner(unitID, attr.NewBearer),
			state.UpdateUnitData(unitID,
				func(data state.UnitData) (state.UnitData, error) {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
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

func validateTransferFungibleToken(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), s)
	if err != nil {
		return err
	}

	if d.locked != 0 {
		return fmt.Errorf("token is locked")
	}

	if d.value != attr.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.value, attr.Value)
	}

	if !bytes.Equal(d.backlink, attr.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
	}

	if !bytes.Equal(attr.TypeID, d.tokenType) {
		return fmt.Errorf("invalid type identifier: expected '%s', got '%s'", d.tokenType, attr.TypeID)
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
	return verifyOwnership(bearer, predicates, &transferFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

func getFungibleTokenData(unitID types.UnitID, s *state.State) (predicates.PredicateBytes, *fungibleTokenData, error) {
	if !unitID.HasType(FungibleTokenUnitType) {
		return nil, nil, fmt.Errorf(ErrStrInvalidUnitID)
	}

	u, err := s.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return nil, nil, fmt.Errorf("unit %v does not exist: %w", unitID, err)
		}
		return nil, nil, err
	}
	d, ok := u.Data().(*fungibleTokenData)
	if !ok {
		return nil, nil, fmt.Errorf("unit %v is not fungible token data", unitID)
	}
	return u.Bearer(), d, nil
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
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
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

func (t *TransferFungibleTokenAttributes) GetTypeID() types.UnitID {
	return t.TypeID
}

func (t *TransferFungibleTokenAttributes) SetTypeID(typeID types.UnitID) {
	t.TypeID = typeID
}

func (t *TransferFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return t.InvariantPredicateSignatures
}

func (t *TransferFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	t.InvariantPredicateSignatures = signatures
}

func (t *TransferFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &TransferFungibleTokenAttributes{
		NewBearer:                    t.NewBearer,
		Value:                        t.Value,
		Nonce:                        t.Nonce,
		Backlink:                     t.Backlink,
		TypeID:                       t.TypeID,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
