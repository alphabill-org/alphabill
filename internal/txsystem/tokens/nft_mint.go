package tokens

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/validator/internal/state"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	"github.com/alphabill-org/alphabill/validator/pkg/tree/avl"
	"github.com/fxamacker/cbor/v2"
)

func handleMintNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[MintNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *MintNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := validateMintNonFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid mint non-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		unitID := tx.UnitID()
		h := tx.Hash(options.hashAlgorithm)

		// update state
		if err := options.state.Apply(
			state.AddUnit(unitID, attr.Bearer, newNonFungibleTokenData(attr, h, currentBlockNr))); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateMintNonFungibleToken(tx *types.TransactionOrder, attr *MintNonFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	unitID := tx.UnitID()
	if !unitID.HasType(NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	if len(attr.Name) > maxNameLength {
		return errors.New(ErrStrInvalidNameLength)
	}
	uri := attr.URI
	if uri != "" {
		if len(uri) > uriMaxSize {
			return fmt.Errorf("URI exceeds the maximum allowed size of %v KB", uriMaxSize)
		}
		if !util.IsValidURI(uri) {
			return fmt.Errorf("URI %s is invalid", uri)
		}
	}
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	u, err := s.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("unit %v exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}
	if !attr.NFTTypeID.HasType(NonFungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidTypeID)
	}

	// the transaction request satisfies the predicate obtained by concatenating all the token creation clauses along
	// the type inheritance chain.
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		hashAlgorithm,
		s,
		attr.NFTTypeID,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) types.UnitID {
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

func (m *MintNonFungibleTokenAttributes) GetBearer() []byte {
	return m.Bearer
}

func (m *MintNonFungibleTokenAttributes) SetBearer(bearer []byte) {
	m.Bearer = bearer
}

func (m *MintNonFungibleTokenAttributes) GetNFTTypeID() types.UnitID {
	return m.NFTTypeID
}

func (m *MintNonFungibleTokenAttributes) SetNFTTypeID(nftTypeID types.UnitID) {
	m.NFTTypeID = nftTypeID
}

func (m *MintNonFungibleTokenAttributes) GetName() string {
	return m.Name
}

func (m *MintNonFungibleTokenAttributes) SetName(name string) {
	m.Name = name
}

func (m *MintNonFungibleTokenAttributes) GetURI() string {
	return m.URI
}

func (m *MintNonFungibleTokenAttributes) SetURI(uri string) {
	m.URI = uri
}

func (m *MintNonFungibleTokenAttributes) GetData() []byte {
	return m.Data
}

func (m *MintNonFungibleTokenAttributes) SetData(data []byte) {
	m.Data = data
}

func (m *MintNonFungibleTokenAttributes) GetDataUpdatePredicate() []byte {
	return m.DataUpdatePredicate
}

func (m *MintNonFungibleTokenAttributes) SetDataUpdatePredicate(predicate []byte) {
	m.DataUpdatePredicate = predicate
}

func (m *MintNonFungibleTokenAttributes) GetTokenCreationPredicateSignatures() [][]byte {
	return m.TokenCreationPredicateSignatures
}

func (m *MintNonFungibleTokenAttributes) SetTokenCreationPredicateSignatures(signatures [][]byte) {
	m.TokenCreationPredicateSignatures = signatures
}

func (m *MintNonFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude TokenCreationPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &MintNonFungibleTokenAttributes{
		Bearer:                           m.Bearer,
		NFTTypeID:                        m.NFTTypeID,
		Name:                             m.Name,
		URI:                              m.URI,
		Data:                             m.Data,
		DataUpdatePredicate:              m.DataUpdatePredicate,
		TokenCreationPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
