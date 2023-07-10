package tokens

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/tree/avl"
	"github.com/fxamacker/cbor/v2"
)

func handleMintFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[MintFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Mint Fungible Token tx: %v", tx)
		if err := validateMintFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid mint fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		unitID := tx.UnitID()
		h := tx.Hash(options.hashAlgorithm)

		// update state
		if err := options.state.Apply(
			state.AddUnit(unitID, attr.Bearer, newFungibleTokenData(attr, h, currentBlockNr)),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}}, nil
	}
}

func validateMintFungibleToken(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	unitID := types.UnitID(tx.UnitID())
	if unitID.IsZero(hashAlgorithm.Size()) {
		return errors.New(ErrStrUnitIDIsZero)
	}
	u, err := s.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("unit with id %v already exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if attr.Value == 0 {
		return errors.New("token must have value greater than zero")
	}

	// existence of the parent type is checked by the getChainedPredicates
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		hashAlgorithm,
		s,
		attr.TypeID,
		func(d *fungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *fungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	sigBytes, err := tx.Payload.BytesWithAttributeSigBytes(attr)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, attr.TokenCreationPredicateSignatures, sigBytes)
}

func (m *MintFungibleTokenAttributes) GetBearer() []byte {
	return m.Bearer
}

func (m *MintFungibleTokenAttributes) SetBearer(bearer []byte) {
	m.Bearer = bearer
}

func (m *MintFungibleTokenAttributes) GetTypeID() []byte {
	return m.TypeID
}

func (m *MintFungibleTokenAttributes) SetTypeID(typeID []byte) {
	m.TypeID = typeID
}

func (m *MintFungibleTokenAttributes) GetValue() uint64 {
	return m.Value
}

func (m *MintFungibleTokenAttributes) SetValue(value uint64) {
	m.Value = value
}

func (m *MintFungibleTokenAttributes) GetTokenCreationPredicateSignatures() [][]byte {
	return m.TokenCreationPredicateSignatures
}

func (m *MintFungibleTokenAttributes) SetTokenCreationPredicateSignatures(signatures [][]byte) {
	m.TokenCreationPredicateSignatures = signatures
}

func (m *MintFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016
	signatureAttr := &MintFungibleTokenAttributes{
		Bearer:                           m.Bearer,
		TypeID:                           m.TypeID,
		Value:                            m.Value,
		TokenCreationPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
