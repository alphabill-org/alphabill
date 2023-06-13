package tokens

import (
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

func handleMintNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[MintNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *MintNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Mint Non-Fungible Token tx: %v", tx)
		if err := validateMintNonFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid mint none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.AddItem(unitID, attr.Bearer, newNonFungibleTokenData(attr, h, currentBlockNr), h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateMintNonFungibleToken(tx *types.TransactionOrder, attr *MintNonFungibleTokenAttributes, state *rma.Tree) error {
	unitID := util.BytesToUint256(tx.UnitID())
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
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
	u, err := state.GetUnit(unitID)
	if u != nil {
		return fmt.Errorf("unit %v exists", unitID)
	}
	if !errors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	nftTypeID := util.BytesToUint256(attr.NFTTypeID)
	if nftTypeID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}

	// the transaction request satisfies the predicate obtained by concatenating all the token creation clauses along
	// the type inheritance chain.
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		state,
		nftTypeID,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
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

func (m *MintNonFungibleTokenAttributes) GetNFTTypeID() []byte {
	return m.NFTTypeID
}

func (m *MintNonFungibleTokenAttributes) SetNFTTypeID(nftTypeID []byte) {
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
