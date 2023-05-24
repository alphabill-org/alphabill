package tokens

import (
	gocrypto "crypto"
	"fmt"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	hasherUtil "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const validNFTURI = "https://alphabill.org/nft"

var (
	parent1Identifier = uint256.NewInt(1)
	parent2Identifier = uint256.NewInt(2)
	unitIdentifier    = uint256.NewInt(10)
	nftTypeID         = test.RandomBytes(32)
	nftName           = fmt.Sprintf("Long name for %v", unitIdentifier)
)

var (
	unitID                   = []byte{1}
	symbol                   = "TEST"
	name                     = "Long name for " + symbol
	parentTypeId             = []byte{3}
	subTypeCreationPredicate = []byte{4}
	tokenCreationPredicate   = []byte{5}
	invariantPredicate       = []byte{6}
	dataUpdatePredicate      = []byte{7}
	bearer                   = []byte{10}
	data                     = []byte{12}
	updatedData              = []byte{0, 12}
	backlink                 = []byte{17}
)

func TestNewTokenTxSystem_NilSystemIdentifier(t *testing.T) {
	txs, err := New(WithSystemIdentifier(nil))
	require.ErrorContains(t, err, ErrStrSystemIdentifierIsNil)
	require.Nil(t, txs)
}

func TestNewTokenTxSystem_StateIsNil(t *testing.T) {
	txs, err := New(WithState(nil))
	require.ErrorContains(t, err, ErrStrStateIsNil)
	require.Nil(t, txs)
}

func TestExecuteCreateNFTType_WithoutParentID(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
		}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := txs.GetState().GetUnit(unitIdentifier)
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenTypeData{}, u.Data)
	data := u.Data.(*nonFungibleTokenTypeData)
	require.Equal(t, zeroSummaryValue, data.Value())
	require.Equal(t, symbol, data.symbol)
	require.Equal(t, uint256.NewInt(0), data.parentTypeId)
	require.Equal(t, subTypeCreationPredicate, data.subTypeCreationPredicate)
	require.Equal(t, tokenCreationPredicate, data.tokenCreationPredicate)
	require.Equal(t, invariantPredicate, data.invariantPredicate)
	require.Equal(t, dataUpdatePredicate, data.dataUpdatePredicate)
}

func TestExecuteCreateNFTType_WithParentID(t *testing.T) {
	txs := newTokenTxSystem(t)
	createParentTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent1Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
		}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	sm, err := txs.Execute(createParentTx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeID:                       util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
				SubTypeCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
	parent1SubTypeCreationPredicate := script.PredicatePayToPublicKeyHashDefault(hasherUtil.Sum256(parent2PubKey))

	// parent2 and child together can create a sub-type because SubTypeCreationPredicate are concatenated (ownerProof must contain both signatures)
	parent2SubTypeCreationPredicate := script.PredicatePayToPublicKeyHashDefault(hasherUtil.Sum256(childPublicKey))
	parent2SubTypeCreationPredicate[0] = script.StartByte // verify parent1SubTypeCreationPredicate signature verification result

	txs := newTokenTxSystem(t)

	// create parent1 type
	createParent1Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent1Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: parent1SubTypeCreationPredicate,
		}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	sm, err := txs.Execute(createParent1Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// create parent2 type
	unsignedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent2Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                   symbol,
				ParentTypeID:             util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
			},
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, p2pkhPredicate := signTx(t, unsignedCreateParent2Tx, parent2Signer, parent2PubKey)

	signedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent2Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeID:                       util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate:           parent2SubTypeCreationPredicate,
				SubTypeCreationPredicateSignatures: [][]byte{p2pkhPredicate},
			},
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	sm, err = txs.Execute(signedCreateParent2Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// create child sub-type
	unsignedChildTxAttributes := &CreateNonFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		ParentTypeID:             util.Uint256ToBytes(parent2Identifier),
		SubTypeCreationPredicate: script.PredicateAlwaysFalse(), // no sub-types
	}
	createChildTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	sigBytes, err := createChildTx.PayloadBytes()
	require.NoError(t, err)

	signature, err := childSigner.SignBytes(sigBytes)
	require.NoError(t, err)
	signature2, err := parent2Signer.SignBytes(sigBytes)
	require.NoError(t, err)

	// child owner proof must satisfy parent1 & parent2 SubTypeCreationPredicates
	unsignedChildTxAttributes.SubTypeCreationPredicateSignatures = [][]byte{
		script.PredicateArgumentPayToPublicKeyHashDefault(signature, childPublicKey), // parent2 p2pkhPredicate argument
		script.PredicateArgumentPayToPublicKeyHashDefault(signature2, parent2PubKey), // parent1 p2pkhPredicate argument
	}
	createChildTx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	sm, err = txs.Execute(createChildTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
}

func TestExecuteCreateNFTType_UnitTypeIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(uint256.NewInt(0))),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrUnitIDIsZero)
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_UnitIDExists(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	sm, err = txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("unit %v exists", unitIdentifier))
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_ParentDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			ParentTypeID:             util.Uint256ToBytes(parent1Identifier),
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(parent1Identifier)))
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_InvalidParentType(t *testing.T) {
	txs := newTokenTxSystem(t)
	require.NoError(t, txs.GetState().AtomicUpdate(rma.AddItem(parent1Identifier, script.PredicateAlwaysTrue(), &mockUnitData{}, []byte{})))
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			ParentTypeID:             util.Uint256ToBytes(parent1Identifier),
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("unit %v data is not of type %T", parent1Identifier, &nonFungibleTokenTypeData{}))
}

func TestExecuteCreateNFTType_InvalidSystemIdentifier(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid system identifier")
}

func TestExecuteCreateNFTType_InvalidTxType(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "unknown transaction type")
}

func TestRevertTransaction_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	txs.Revert()

	_, err = txs.GetState().GetUnit(unitIdentifier)
	require.ErrorContains(t, err, fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(unitIdentifier)))
}

func TestExecuteCreateNFTType_InvalidSymbolLength(t *testing.T) {
	s := "♥ Alphabill ♥"
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: s,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Name:   n,
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidNameLength)
}

func TestExecuteCreateNFTType_InvalidIconTypeLength(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Icon:   &Icon{Type: invalidIconType, Data: []byte{1, 2, 3}},
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidIconTypeLength)
}

func TestExecuteCreateNFTType_InvalidIconDataLength(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Icon:   &Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidIconDataLength)
}

func TestMintNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(nftTypeID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
			DataUpdatePredicate:      script.PredicateAlwaysTrue(),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	_, err := txs.Execute(tx)
	require.NoError(t, err)
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NFTTypeID:                        nftTypeID,
			Name:                             nftName,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err = txs.Execute(tx)
	require.NoError(t, err)
	u, err := txs.GetState().GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	txHash := tx.Hash(gocrypto.SHA256)
	require.Equal(t, txHash, u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, nftName, d.name)
	require.Equal(t, []byte{10}, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Equal(t, script.PredicateAlwaysTrue(), d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, txHash, d.backlink)
}

func TestMintNFT_UnitIDIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(util.Uint256ToBytes(uint256.NewInt(0))),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrUnitIDIsZero)
}

func TestMintNFT_UnitIDExists(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
			DataUpdatePredicate:      script.PredicateAlwaysTrue(),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	_, err := txs.Execute(tx)
	require.NoError(t, err)
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NFTTypeID:                        nftTypeID,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err = txs.Execute(tx)
	require.NoError(t, err)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "unit 1 exist")
}

func TestMintNFT_NFTTypeIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	idBytes := util.Uint256ToBytes(uint256.NewInt(110))
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NFTTypeID:                        idBytes,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %X does not exist", idBytes))
}

func TestMintNFT_NameLengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Name: test.RandomString(maxNameLength + 1),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI: test.RandomString(4097),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI: "invalid_uri",
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI:  validNFTURI,
			Data: test.RandomBytes(dataMaxSize + 1),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "data exceeds the maximum allowed size of 65536 KB")
}

func TestMintNFT_NFTTypeDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			URI:       validNFTURI,
			Data:      []byte{0, 0, 0, 0},
			NFTTypeID: []byte{0, 0, 0, 1},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "item 0000000000000000000000000000000000000000000000000000000000000001 does not exist")
}

func TestTransferNFT_UnitDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     test.RandomBytes(32),
			InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysTrue()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "item 0000000000000000000000000000000000000000000000000000000000000001 does not exist")
}

func TestTransferNFT_UnitIsNotNFT(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     test.RandomBytes(32),
			InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysTrue()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "unit 10 is not a non-fungible token type")
}

func TestTransferNFT_InvalidBacklink(t *testing.T) {
	txs := newTokenTxSystem(t)
	createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     []byte{1},
			InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid backlink")
}

func TestTransferNFT_InvalidTypeID(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    test.RandomBytes(32),
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid type identifier")
}

func TestTransferNFT_EmptyTypeID(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid type identifier")
}

func createClientMetadata() *types.ClientMetadata {
	return &types.ClientMetadata{
		Timeout:           1000,
		MaxTransactionFee: 10,
		FeeCreditRecordID: util.Uint256ToBytes(feeCreditID),
	}
}

func TestTransferNFT_InvalidPredicateFormat(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID,
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid script format")
}

func TestTransferNFT_InvalidSignature(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID,
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "script execution result yielded false or non-clean stack")
}

func TestTransferNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID,
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	u, err := txs.GetState().GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, nftName, d.name)
	require.Equal(t, []byte{10}, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Equal(t, script.PredicateAlwaysTrue(), d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, script.PredicateAlwaysTrue(), []byte(u.Bearer))
}

func TestTransferNFT_BurnedBearerMustFail(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT, set bearer to unspendable predicate
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID,
			NewBearer:                    script.PredicateAlwaysFalse(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	u, err := txs.GetState().GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	require.Equal(t, script.PredicateAlwaysFalse(), []byte(u.Bearer))

	// the token must be considered as burned and not transferable
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID,
			NewBearer:                    []byte{script.StartByte},
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err = txs.Execute(tx)
	require.ErrorIs(t, err, script.ErrScriptResultFalse)
}

func TestUpdateNFT_DataLengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(dataMaxSize + 1),
			Backlink: test.RandomBytes(32),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(0),
			Backlink: test.RandomBytes(32),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "item 0000000000000000000000000000000000000000000000000000000000000001 does not exist")
}

func TestUpdateNFT_UnitIsNotNFT(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: test.RandomBytes(32),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "unit 10 is not a non-fungible token type")
}

func TestUpdateNFT_InvalidBacklink(t *testing.T) {
	txs := newTokenTxSystem(t)
	createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: []byte{1},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid backlink")
}

func TestUpdateNFT_InvalidPredicateFormat(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:                 test.RandomBytes(10),
			Backlink:             tx.Hash(gocrypto.SHA256),
			DataUpdateSignatures: [][]byte{script.PredicateArgumentEmpty(), {}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid script format")
}

func TestUpdateNFT_InvalidSignature(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:                 test.RandomBytes(10),
			Backlink:             tx.Hash(gocrypto.SHA256),
			DataUpdateSignatures: [][]byte{script.PredicateAlwaysTrue(), script.PredicateAlwaysFalse()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "script execution result yielded false or non-clean stack")
}

func TestUpdateNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// update NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeUpdateNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Backlink:             tx.Hash(gocrypto.SHA256),
			Data:                 updatedData,
			DataUpdateSignatures: [][]byte{script.PredicateArgumentEmpty(), script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	u, err := txs.GetState().GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, nftName, d.name)
	require.Equal(t, updatedData, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Equal(t, script.PredicateAlwaysTrue(), d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, script.PredicateAlwaysTrue(), []byte(u.Bearer))
}

func createNFTTypeAndMintToken(t *testing.T, txs *txsystem.GenericTxSystem, nftTypeID []byte, nftID []byte) *types.TransactionOrder {
	// create NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithUnitId(nftTypeID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
			DataUpdatePredicate:      script.PredicateAlwaysTrue(),
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	_, err := txs.Execute(tx)
	require.NoError(t, err)

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeMintNFT),
		testtransaction.WithUnitId(nftID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NFTTypeID:                        nftTypeID,
			Name:                             nftName,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	_, err = txs.Execute(tx)
	require.NoError(t, err)
	return tx
}

type mockUnitData struct{}

func (m mockUnitData) AddToHasher(hash.Hash) {}

func (m mockUnitData) Value() rma.SummaryValue { return zeroSummaryValue }

func createSigner(t *testing.T) (crypto.Signer, []byte) {
	t.Helper()
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)

	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	return signer, pubKey
}

func signTx(t *testing.T, tx *types.TransactionOrder, signer crypto.Signer, pubKey []byte) ([]byte, []byte) {
	sigBytes, err := tx.PayloadBytes()
	require.NoError(t, err)
	signature, err := signer.SignBytes(sigBytes)
	require.NoError(t, err)
	return signature, script.PredicateArgumentPayToPublicKeyHashDefault(signature, pubKey)
}

func newTokenTxSystem(t *testing.T) *txsystem.GenericTxSystem {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	state := rma.NewWithSHA256()
	require.NoError(t, state.AtomicUpdate(rma.AddItem(feeCreditID, script.PredicateAlwaysTrue(), &fc.FeeCreditRecord{
		Balance: 100,
		Hash:    make([]byte, 32),
		Timeout: 1000,
	}, make([]byte, 32))))
	state.Commit()
	txs, err := New(
		WithTrustBase(map[string]crypto.Verifier{"test": verifier}),
		WithState(state),
	)
	require.NoError(t, err)
	return txs
}
