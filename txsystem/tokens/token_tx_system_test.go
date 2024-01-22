package tokens

import (
	gocrypto "crypto"
	"fmt"
	"hash"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	hasherUtil "github.com/alphabill-org/alphabill/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

const validNFTURI = "https://alphabill.org/nft"

var (
	parent1Identifier = NewNonFungibleTokenTypeID(nil, []byte{1})
	parent2Identifier = NewNonFungibleTokenTypeID(nil, []byte{2})
	nftTypeID1        = NewNonFungibleTokenTypeID(nil, []byte{10})
	nftTypeID2        = NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
	nftName           = fmt.Sprintf("Long name for %s", nftTypeID1)
)

var (
	unitID                   = NewNonFungibleTokenID(nil, []byte{1})
	symbol                   = "TEST"
	subTypeCreationPredicate = []byte{4}
	tokenCreationPredicate   = []byte{5}
	invariantPredicate       = []byte{6}
	dataUpdatePredicate      = []byte{7}
	updatedData              = []byte{0, 12}
	//bearer                   = []byte{10}
	//data                     = []byte{12}
	//backlink                 = []byte{17}
)

func TestNewTokenTxSystem_NilSystemIdentifier(t *testing.T) {
	txs, err := NewTxSystem(nil, WithSystemIdentifier(0))
	require.ErrorContains(t, err, ErrStrInvalidSystemID)
	require.Nil(t, txs)
}

func TestNewTokenTxSystem_StateIsNil(t *testing.T) {
	txs, err := NewTxSystem(nil, WithState(nil))
	require.ErrorContains(t, err, ErrStrStateIsNil)
	require.Nil(t, txs)
}

func TestExecuteCreateNFTType_WithoutParentID(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := txs.State().GetUnit(nftTypeID1, false)
	require.NoError(t, err)
	require.IsType(t, &NonFungibleTokenTypeData{}, u.Data())
	d := u.Data().(*NonFungibleTokenTypeData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, symbol, d.Symbol)
	require.Nil(t, d.ParentTypeId)
	require.Equal(t, subTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.Equal(t, tokenCreationPredicate, d.TokenCreationPredicate)
	require.Equal(t, invariantPredicate, d.InvariantPredicate)
	require.Equal(t, dataUpdatePredicate, d.DataUpdatePredicate)
}

func TestExecuteCreateNFTType_WithParentID(t *testing.T) {
	txs := newTokenTxSystem(t)
	createParentTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(parent1Identifier),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(createParentTx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeID:                       parent1Identifier,
				SubTypeCreationPredicate:           templates.AlwaysFalseBytes(),
				SubTypeCreationPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
}

func TestExecuteCreateNFTType_InheritanceChainWithP2PKHPredicates(t *testing.T) {
	// Inheritance Chain: parent1Identifier <- parent2Identifier <- unitIdentifier
	parent2Signer, parent2PubKey := createSigner(t)
	childSigner, childPublicKey := createSigner(t)

	// only parent2 can create sub-types from parent1
	parent1SubTypeCreationPredicate := templates.NewP2pkh256BytesFromKeyHash(hasherUtil.Sum256(parent2PubKey))

	// parent2 and child together can create a sub-type because SubTypeCreationPredicate are concatenated (ownerProof must contain both signatures)
	parent2SubTypeCreationPredicate := templates.NewP2pkh256BytesFromKeyHash(hasherUtil.Sum256(childPublicKey))

	txs := newTokenTxSystem(t)

	// create parent1 type
	createParent1Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(parent1Identifier),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: parent1SubTypeCreationPredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(createParent1Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// create parent2 type
	unsignedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(parent2Identifier),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
			},
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, p2pkhPredicate := signTx(t, unsignedCreateParent2Tx, parent2Signer, parent2PubKey)

	signedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(parent2Identifier),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeID:                       parent1Identifier,
				SubTypeCreationPredicate:           parent2SubTypeCreationPredicate,
				SubTypeCreationPredicateSignatures: [][]byte{p2pkhPredicate},
			},
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err = txs.Execute(signedCreateParent2Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// create child sub-type
	unsignedChildTxAttributes := &CreateNonFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		ParentTypeID:             parent2Identifier,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(), // no sub-types
	}
	createChildTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithFeeProof(nil),
	)

	sigBytes, err := createChildTx.PayloadBytes()
	require.NoError(t, err)

	signature, err := childSigner.SignBytes(sigBytes)
	require.NoError(t, err)
	signature2, err := parent2Signer.SignBytes(sigBytes)
	require.NoError(t, err)

	// child owner proof must satisfy parent1 & parent2 SubTypeCreationPredicates
	unsignedChildTxAttributes.SubTypeCreationPredicateSignatures = [][]byte{
		templates.NewP2pkh256SignatureBytes(signature, childPublicKey), // parent2 p2pkhPredicate argument
		templates.NewP2pkh256SignatureBytes(signature2, parent2PubKey), // parent1 p2pkhPredicate argument
	}
	createChildTx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err = txs.Execute(createChildTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
}

func TestExecuteCreateNFTType_UnitIDIsNil(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nil),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_UnitIDHasWrongType(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_ParentTypeIDHasWrongType(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{ParentTypeID: unitID}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_UnitIDExists(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	sm, err = txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("unit %s exists", nftTypeID1))
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_ParentDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			ParentTypeID:             parent1Identifier,
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", parent1Identifier))
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_InvalidParentType(t *testing.T) {
	txs := newTokenTxSystem(t)
	require.NoError(t, txs.GetState().Apply(state.AddUnit(parent1Identifier, templates.AlwaysTrueBytes(), &mockUnitData{})))
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			ParentTypeID:             parent1Identifier,
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("unit %s data is not of type %T", parent1Identifier, &NonFungibleTokenTypeData{}))
}

func TestExecuteCreateNFTType_InvalidSystemIdentifier(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(0),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid system identifier")
}

func TestExecuteCreateNFTType_InvalidTxType(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "unknown transaction type")
}

func TestRevertTransaction_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{ParentTypeID: nil}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	txs.Revert()

	_, err = txs.State().GetUnit(nftTypeID1, false)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", nftTypeID1))
}

func TestExecuteCreateNFTType_InvalidSymbolLength(t *testing.T) {
	s := "♥ Alphabill ♥"
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: s,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidSymbolLength)
}

func TestExecuteCreateNFTType_InvalidNameLength(t *testing.T) {
	n := "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill♥♥"
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Name:   n,
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidNameLength)
}

func TestExecuteCreateNFTType_InvalidIconTypeLength(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Icon:   &Icon{Type: invalidIconType, Data: []byte{1, 2, 3}},
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidIconTypeLength)
}

func TestExecuteCreateNFTType_InvalidIconDataLength(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Icon:   &Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidIconDataLength)
}

func TestMintNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nftTypeID2),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenCreationPredicate:   templates.AlwaysTrueBytes(),
			InvariantPredicate:       templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	_, err := txs.Execute(tx)
	require.NoError(t, err)
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
			NFTTypeID:                        nftTypeID2,
			Name:                             nftName,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              templates.AlwaysTrueBytes(),
			TokenCreationPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.NoError(t, err)

	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	txHash := tx.Hash(gocrypto.SHA256)
	require.IsType(t, &NonFungibleTokenData{}, u.Data())
	d := u.Data().(*NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, []byte{10}, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, txHash, d.Backlink)
}

func TestMintNFT_UnitIDIsNil(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(nil),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
}

func TestMintNFT_UnitIDHasWrongType(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
}

func TestMintNFT_TypeIDHasWrongType(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{NFTTypeID: unitID}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidTypeID)
}

func TestMintNFT_UnitIDExists(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID2),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenCreationPredicate:   templates.AlwaysTrueBytes(),
			InvariantPredicate:       templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	_, err := txs.Execute(tx)
	require.NoError(t, err)
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
			NFTTypeID:                        nftTypeID2,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              templates.AlwaysTrueBytes(),
			TokenCreationPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.NoError(t, err)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("unit %s exists", unitID))
}

func TestMintNFT_NFTTypeIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	idBytes := NewNonFungibleTokenTypeID(nil, []byte{110})
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
			NFTTypeID:                        idBytes,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              templates.AlwaysTrueBytes(),
			TokenCreationPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", idBytes))
}

func TestMintNFT_NameLengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Name: test.RandomString(maxNameLength + 1),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidNameLength)
}

func TestMintNFT_URILengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI: test.RandomString(4097),
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "URI exceeds the maximum allowed size of 4096 KB")
}

func TestMintNFT_URIFormatIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI: "invalid_uri",
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "URI invalid_uri is invalid")
}

func TestMintNFT_DataLengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI:  validNFTURI,
			Data: test.RandomBytes(dataMaxSize + 1),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "data exceeds the maximum allowed size of 65536 KB")
}

func TestMintNFT_NFTTypeDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	typeID := NewNonFungibleTokenTypeID(nil, []byte{1})
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI:       validNFTURI,
			Data:      []byte{0, 0, 0, 0},
			NFTTypeID: typeID,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", typeID))
}

func TestTransferNFT_UnitDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     test.RandomBytes(32),
			InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", unitID))
}

func TestTransferNFT_UnitIsNotNFT(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     test.RandomBytes(32),
			InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "invalid unit ID")
}

func TestTransferNFT_InvalidBacklink(t *testing.T) {
	txs := newTokenTxSystem(t)
	createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     []byte{1},
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid backlink")
}

func TestTransferNFT_InvalidTypeID(t *testing.T) {
	txs := newTokenTxSystem(t)
	txr := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    NewFungibleTokenTypeID(nil, test.RandomBytes(32)),
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     txr.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid type identifier")
}

func TestTransferNFT_EmptyTypeID(t *testing.T) {
	txs := newTokenTxSystem(t)
	txr := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     txr.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid type identifier")
}

func createClientMetadata() *types.ClientMetadata {
	return &types.ClientMetadata{
		Timeout:           1000,
		MaxTransactionFee: 10,
		FeeCreditRecordID: feeCreditID,
	}
}

func TestTransferNFT_InvalidPredicateFormat(t *testing.T) {
	txs := newTokenTxSystem(t)
	txr := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer NFT from 'always true' to 'p2pkh'
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    test.RandomBytes(32), // invalid bearer
			Nonce:                        test.RandomBytes(32),
			Backlink:                     txr.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.AlwaysTrueArgBytes()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.AlwaysTrueArgBytes()),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "invalid predicate: failed to decode predicate")
}

func TestTransferNFT_InvalidSignature(t *testing.T) {
	txs := newTokenTxSystem(t)
	txr := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer with invalid signature
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     txr.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.AlwaysTrueArgBytes()),
		testtransaction.WithOwnerProof(test.RandomBytes(2)), // invalid signature
	)
	_, err := txs.Execute(tx)

	require.ErrorContains(t, err, "invalid predicate")
}

func TestTransferNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	require.IsType(t, &NonFungibleTokenData{}, u.Data())
	d := u.Data().(*NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, []byte{10}, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.Backlink)
	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Bearer()))
}

func TestTransferNFT_BurnedBearerMustFail(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer NFT, set bearer to unspendable predicate
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysFalseBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	require.IsType(t, &NonFungibleTokenData{}, u.Data())
	require.EqualValues(t, templates.AlwaysFalseBytes(), []byte(u.Bearer()))

	// the token must be considered as burned and not transferable
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysFalseBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "always false")
}

func TestTransferNFT_LockedToken(t *testing.T) {
	txs := newTokenTxSystem(t)
	mintTx := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeLockToken),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&LockTokenAttributes{
			LockStatus:                   1,
			Backlink:                     mintTx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithOwnerProof(nil),
	)
	_, err := txs.Execute(lockTx)
	require.NoError(t, err)

	// verify unit was locked
	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	tokenData := u.Data().(*NonFungibleTokenData)
	require.EqualValues(t, 1, tokenData.Locked)

	// update nft
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     lockTx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)

	// verify token was locked
	require.ErrorContains(t, err, "token is locked")
}

func TestUpdateNFT_DataLengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(dataMaxSize + 1),
			Backlink: test.RandomBytes(32),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "data exceeds the maximum allowed size of 65536 KB")
}

func TestUpdateNFT_UnitDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(0),
			Backlink: test.RandomBytes(32),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", unitID))
}

func TestUpdateNFT_UnitIsNotNFT(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(nftTypeID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: test.RandomBytes(32),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "invalid unit ID")
}

func TestUpdateNFT_LockedToken(t *testing.T) {
	txs := newTokenTxSystem(t)
	mintTx := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeLockToken),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&LockTokenAttributes{
			LockStatus:                   1,
			Backlink:                     mintTx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithOwnerProof(nil),
	)
	_, err := txs.Execute(lockTx)
	require.NoError(t, err)

	// verify unit was locked
	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	tokenData := u.Data().(*NonFungibleTokenData)
	require.EqualValues(t, 1, tokenData.Locked)

	// update nft
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: []byte{1},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)

	// verify token was locked
	require.ErrorContains(t, err, "token is locked")
}

func TestUpdateNFT_InvalidBacklink(t *testing.T) {
	txs := newTokenTxSystem(t)
	createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: []byte{1},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid backlink")
}

func TestUpdateNFT_InvalidSignature(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID2, nil)
	require.NotNil(t, tx)

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
			NFTTypeID:                        nftTypeID2,
			Name:                             nftName,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
			TokenCreationPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:                 test.RandomBytes(10),
			Backlink:             tx.Hash(gocrypto.SHA256),
			DataUpdateSignatures: [][]byte{templates.AlwaysTrueBytes(), templates.AlwaysFalseBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "invalid predicate")
}

func TestUpdateNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// update NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Backlink:             tx.Hash(gocrypto.SHA256),
			Data:                 updatedData,
			DataUpdateSignatures: [][]byte{nil, nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	require.IsType(t, &NonFungibleTokenData{}, u.Data())
	d := u.Data().(*NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, updatedData, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.Backlink)
	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Bearer()))
}

// Test LockFC -> UnlockFC
func TestExecute_LockFeeCreditTxs_OK(t *testing.T) {
	txs := newTokenTxSystem(t)
	s := txs.GetState()

	err := txs.BeginBlock(1)
	require.NoError(t, err)

	// lock fee credit record
	lockFCAttr := testutils.NewLockFCAttr(testutils.WithLockFCBacklink(make([]byte, 32)))
	lockFC := testutils.NewLockFC(t, lockFCAttr,
		testtransaction.WithUnitId(feeCreditID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
	)
	sm, err := txs.Execute(lockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify unit was locked
	u, err := s.GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.True(t, fcr.IsLocked())

	// unlock fee credit record
	unlockFCAttr := testutils.NewUnlockFCAttr(testutils.WithUnlockFCBacklink(lockFC.Hash(gocrypto.SHA256)))
	unlockFC := testutils.NewUnlockFC(t, unlockFCAttr,
		testtransaction.WithUnitId(feeCreditID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
	)
	sm, err = txs.Execute(unlockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify unit was unlocked
	fcrUnit, err := s.GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcr, ok = fcrUnit.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.False(t, fcr.IsLocked())
}

func createNFTTypeAndMintToken(t *testing.T, txs *txsystem.GenericTxSystem, nftTypeID types.UnitID, nftID types.UnitID) *types.TransactionOrder {
	// create NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenCreationPredicate:   templates.AlwaysTrueBytes(),
			InvariantPredicate:       templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithFeeProof(nil),
	)

	_, err := txs.Execute(tx)
	require.NoError(t, err)

	if nftID != nil {
		// mint NFT
		tx = testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(PayloadTypeMintNFT),
			testtransaction.WithUnitId(nftID),
			testtransaction.WithSystemID(DefaultSystemIdentifier),
			testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
				Bearer:                           templates.AlwaysTrueBytes(),
				NFTTypeID:                        nftTypeID,
				Name:                             nftName,
				URI:                              validNFTURI,
				Data:                             []byte{10},
				DataUpdatePredicate:              templates.AlwaysTrueBytes(),
				TokenCreationPredicateSignatures: [][]byte{nil},
			}),
			testtransaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           1000,
				MaxTransactionFee: 10,
				FeeCreditRecordID: feeCreditID,
			}),
			testtransaction.WithFeeProof(nil),
		)
		_, err = txs.Execute(tx)
		require.NoError(t, err)
	}
	return tx
}

type mockUnitData struct{}

func (m mockUnitData) Write(hash.Hash) error { return nil }

func (m mockUnitData) SummaryValueInput() uint64 {
	return 0
}

func (m mockUnitData) Copy() state.UnitData {
	return mockUnitData{}
}

func createSigner(t *testing.T) (abcrypto.Signer, []byte) {
	t.Helper()
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)

	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	return signer, pubKey
}

func signTx(t *testing.T, tx *types.TransactionOrder, signer abcrypto.Signer, pubKey []byte) ([]byte, []byte) {
	sigBytes, err := tx.PayloadBytes()
	require.NoError(t, err)
	signature, err := signer.SignBytes(sigBytes)
	require.NoError(t, err)
	return signature, templates.NewP2pkh256SignatureBytes(signature, pubKey)
}

func newTokenTxSystem(t *testing.T) *txsystem.GenericTxSystem {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(state.AddUnit(feeCreditID, templates.AlwaysTrueBytes(), &unit.FeeCreditRecord{
		Balance:  100,
		Backlink: make([]byte, 32),
		Timeout:  1000,
	})))
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  1,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}))

	txs, err := NewTxSystem(
		logger.New(t),
		WithTrustBase(map[string]abcrypto.Verifier{"test": verifier}),
		WithState(s),
	)
	require.NoError(t, err)
	return txs
}
