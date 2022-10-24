package tokens

import (
	gocrypto "crypto"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/util"
	"hash"
	"math/rand"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/crypto"
	hasher "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const validNFTURI = "https://alphabill.org/nft"

var (
	parent1Identifier = uint256.NewInt(1)
	parent2Identifier = uint256.NewInt(2)
	unitIdentifier    = uint256.NewInt(10)
	nftTypeID         = []byte{100}
)

func TestNewTokenTxSystem_DefaultOptions(t *testing.T) {
	txs, err := New()
	require.NoError(t, err)
	require.Equal(t, gocrypto.SHA256, txs.hashAlgorithm)
	require.Equal(t, DefaultTokenTxSystemIdentifier, txs.systemIdentifier)

	require.NotNil(t, txs.state)
	state, err := txs.State()
	require.NoError(t, err)
	require.Equal(t, make([]byte, gocrypto.SHA256.Size()), state.Root())
	require.Equal(t, zeroSummaryValue.Bytes(), state.Summary())
}

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

func TestNewTokenTxSystem_OverrideDefaultOptions(t *testing.T) {
	systemIdentifier := []byte{0, 0, 0, 7}
	txs, err := New(WithSystemIdentifier(systemIdentifier), WithHashAlgorithm(gocrypto.SHA512))
	require.NoError(t, err)
	require.Equal(t, gocrypto.SHA512, txs.hashAlgorithm)
	require.Equal(t, systemIdentifier, txs.systemIdentifier)

	require.NotNil(t, txs.state)
	state, err := txs.State()
	require.NoError(t, err)
	require.Equal(t, make([]byte, gocrypto.SHA512.Size()), state.Root())
	require.Equal(t, zeroSummaryValue.Bytes(), state.Summary())
}

func TestExecuteCreateNFTType_WithoutParentID(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
		}),
	)

	require.NoError(t, txs.Execute(tx))
	u, err := txs.state.GetUnit(unitIdentifier)
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
	createParentTx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent1Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
		}),
	)

	require.NoError(t, txs.Execute(createParentTx))
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                            symbol,
				ParentTypeId:                      util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate:          script.PredicateAlwaysFalse(),
				SubTypeCreationPredicateSignature: script.PredicateArgumentEmpty(),
			},
		),
	)
	require.NoError(t, txs.Execute(tx))
}

func TestExecuteCreateNFTType_InheritanceChainWithP2PKHPredicates(t *testing.T) {
	// Inheritance Chain: parent1Identifier <- parent2Identifier <- unitIdentifier
	parent2Signer, parent2PubKey := createSigner(t)
	childSigner, childPublicKey := createSigner(t)

	// only parent2 can create sub-types from parent1
	parent1SubTypeCreationPredicate := script.PredicatePayToPublicKeyHashDefault(hasher.Sum256(parent2PubKey))

	// parent2 and child together can create a sub-type because SubTypeCreationPredicate are concatenated (ownerProof must contain both signatures)
	parent2SubTypeCreationPredicate := script.PredicatePayToPublicKeyHashDefault(hasher.Sum256(childPublicKey))
	parent2SubTypeCreationPredicate[0] = script.OpVerify // verify parent1SubTypeCreationPredicate signature verification result (replace script.StartByte byte with OpVerify)

	txs := newTokenTxSystem(t)

	// create parent1 type
	createParent1Tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent1Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: parent1SubTypeCreationPredicate,
		}),
	)
	require.NoError(t, txs.Execute(createParent1Tx))

	// create parent2 type
	unsignedCreateParent2Tx := testtransaction.NewTransaction(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent2Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                   symbol,
				ParentTypeId:             util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
			},
		),
	)
	signature, p2pkhPredicate := signTx(t, txs, unsignedCreateParent2Tx, parent2Signer, parent2PubKey)

	signedCreateParent2Tx := testtransaction.NewTransaction(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent2Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                            symbol,
				ParentTypeId:                      util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate:          parent2SubTypeCreationPredicate,
				SubTypeCreationPredicateSignature: p2pkhPredicate,
			},
		),
	)

	gtx, err := txs.ConvertTx(signedCreateParent2Tx)
	require.NoError(t, err)
	require.NoError(t, txs.Execute(gtx))

	// create child sub-type
	unsignedChildTxAttributes := &CreateNonFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		ParentTypeId:             util.Uint256ToBytes(parent2Identifier),
		SubTypeCreationPredicate: script.PredicateAlwaysFalse(), // no sub-types
	}
	createChildTx := testtransaction.NewTransaction(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
	)

	gtx, err = txs.ConvertTx(createChildTx)
	require.NoError(t, err)

	signature, err = childSigner.SignBytes(gtx.SigBytes())
	require.NoError(t, err)
	signature2, err := parent2Signer.SignBytes(gtx.SigBytes())
	require.NoError(t, err)

	// child owner proof must satisfy parent1 & parent2 SubTypeCreationPredicates
	unsignedChildTxAttributes.SubTypeCreationPredicateSignature = append(
		script.PredicateArgumentPayToPublicKeyHashDefault(signature, childPublicKey),        // parent2 p2pkhPredicate argument (with script.StartByte byte)
		script.PredicateArgumentPayToPublicKeyHashDefault(signature2, parent2PubKey)[1:]..., // parent1 p2pkhPredicate argument (without script.StartByte byte)
	)
	createChildTx = testtransaction.NewTransaction(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
	)
	gtx, err = txs.ConvertTx(createChildTx)
	require.NoError(t, err)
	require.NoError(t, txs.Execute(gtx))
}

func TestExecuteCreateNFTType_UnitTypeIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(uint256.NewInt(0))),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
	)
	require.ErrorContains(t, txs.Execute(tx), ErrStrUnitIDIsZero)
}

func TestExecuteCreateNFTType_UnitIDExists(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
	)
	require.NoError(t, txs.Execute(tx))
	require.ErrorContains(t, txs.Execute(tx), fmt.Sprintf("unit %v exists", unitIdentifier))
}

func TestExecuteCreateNFTType_ParentDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			ParentTypeId:             util.Uint256ToBytes(parent1Identifier),
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), fmt.Sprintf("item %v does not exist", parent1Identifier))
}

func TestExecuteCreateNFTType_InvalidParentType(t *testing.T) {
	txs := newTokenTxSystem(t)
	txs.state.AddItem(parent1Identifier, script.PredicateAlwaysTrue(), &mockUnitData{}, []byte{})
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			ParentTypeId:             util.Uint256ToBytes(parent1Identifier),
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), fmt.Sprintf("unit %v data is not of type %T", parent1Identifier, &nonFungibleTokenTypeData{}))
}

func TestExecuteCreateNFTType_InvalidSystemIdentifier(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
	)
	require.ErrorContains(t, txs.Execute(tx), "invalid system identifier")
}

func TestExecuteCreateNFTType_InvalidTxType(t *testing.T) {
	txs, err := New(WithSystemIdentifier([]byte{0, 0, 0, 0}))
	require.NoError(t, err)
	tx := testtransaction.RandomGenericBillTransfer(t)
	require.ErrorContains(t, txs.Execute(tx), "unknown tx type")
}

func TestRevertTransaction_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
	)
	require.NoError(t, txs.Execute(tx))
	txs.Revert()
	_, err := txs.state.GetUnit(unitIdentifier)
	require.ErrorContains(t, err, fmt.Sprintf("item %v does not exist", unitIdentifier))
}

func TestExecuteCreateNFTType_InvalidSymbolName(t *testing.T) {
	s := "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥"
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol: s,
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), ErrStrInvalidSymbolName)
}

func TestMintNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(nftTypeID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
			DataUpdatePredicate:      script.PredicateAlwaysTrue(),
		}),
	)

	require.NoError(t, txs.Execute(tx))
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                          script.PredicateAlwaysTrue(),
			NftType:                         nftTypeID,
			Uri:                             validNFTURI,
			Data:                            []byte{10},
			DataUpdatePredicate:             script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
		}),
	)
	require.NoError(t, txs.Execute(tx))
	u, err := txs.state.GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, []byte{10}, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Equal(t, script.PredicateAlwaysTrue(), d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, make([]byte, 32), d.backlink)
}

func TestMintNFT_UnitIDIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(uint256.NewInt(0))),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{}),
	)
	require.ErrorContains(t, txs.Execute(tx), ErrStrUnitIDIsZero)
}

func TestMintNFT_UnitIDExists(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(nftTypeID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
			DataUpdatePredicate:      script.PredicateAlwaysTrue(),
		}),
	)

	require.NoError(t, txs.Execute(tx))
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                          script.PredicateAlwaysTrue(),
			NftType:                         nftTypeID,
			Uri:                             validNFTURI,
			Data:                            []byte{10},
			DataUpdatePredicate:             script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
		}),
	)
	require.NoError(t, txs.Execute(tx))
	require.ErrorContains(t, txs.Execute(tx), "unit 1 exist")
}

func TestMintNFT_NFTTypeIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                          script.PredicateAlwaysTrue(),
			NftType:                         util.Uint256ToBytes(uint256.NewInt(110)),
			Uri:                             validNFTURI,
			Data:                            []byte{10},
			DataUpdatePredicate:             script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "item 110 does not exist")
}

func TestMintNFT_URILengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Uri: randomString(4097),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "URI exceeds the maximum allowed size of 4096 KB")
}

func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:length]
}

func TestMintNFT_URIFormatIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Uri: "invalid_uri",
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "URI invalid_uri is invalid")
}

func TestMintNFT_DataLengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Uri:  validNFTURI,
			Data: test.RandomBytes(dataMaxSize + 1),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "data exceeds the maximum allowed size of 65536 KB")
}

func TestMintNFT_NFTTypeDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Uri:     validNFTURI,
			Data:    []byte{0, 0, 0, 0},
			NftType: []byte{0, 0, 0, 1},
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "item 1 does not exist")
}

func TestTransferNFT_UnitDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                   script.PredicateAlwaysTrue(),
			Nonce:                       test.RandomBytes(32),
			Backlink:                    test.RandomBytes(32),
			InvariantPredicateSignature: script.PredicateAlwaysTrue(),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "item 1 does not exist")
}

func TestTransferNFT_UnitIsNotNFT(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
		}),
	)
	require.NoError(t, txs.Execute(tx))

	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                   script.PredicateAlwaysTrue(),
			Nonce:                       test.RandomBytes(32),
			Backlink:                    test.RandomBytes(32),
			InvariantPredicateSignature: script.PredicateAlwaysTrue(),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "unit 10 is not a non-fungible token type")
}

func TestTransferNFT_InvalidBacklink(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                   script.PredicateAlwaysTrue(),
			Nonce:                       test.RandomBytes(32),
			Backlink:                    []byte{1},
			InvariantPredicateSignature: script.PredicateArgumentEmpty(),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "invalid backlink")
}

func TestTransferNFT_InvalidPredicateFormat(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                   script.PredicateAlwaysTrue(),
			Nonce:                       test.RandomBytes(32),
			Backlink:                    make([]byte, 32),
			InvariantPredicateSignature: []byte{0, 0, 0, 1},
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "invalid script format")
}

func TestTransferNFT_InvalidSignature(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                   script.PredicateAlwaysTrue(),
			Nonce:                       test.RandomBytes(32),
			Backlink:                    make([]byte, 32),
			InvariantPredicateSignature: script.PredicateAlwaysFalse(),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "script execution result yielded false or non-clean stack")
}

func TestTransferNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// transfer NFT
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                   script.PredicateAlwaysTrue(),
			Nonce:                       test.RandomBytes(32),
			Backlink:                    make([]byte, 32),
			InvariantPredicateSignature: script.PredicateArgumentEmpty(),
		}),
	)
	require.NoError(t, txs.Execute(tx))

	u, err := txs.state.GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, []byte{10}, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Nil(t, d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, script.PredicateAlwaysTrue(), []byte(u.Bearer))
}

func TestUpdateNFT_DataLengthIsInvalid(t *testing.T) {
	txs := newTokenTxSystem(t)
	createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(dataMaxSize + 1),
			Backlink: test.RandomBytes(32),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "data exceeds the maximum allowed size of 65536 KB")
}

func TestUpdateNFT_UnitDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(0),
			Backlink: test.RandomBytes(32),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "item 1 does not exist")
}

func TestUpdateNFT_UnitIsNotNFT(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
		}),
	)
	require.NoError(t, txs.Execute(tx))

	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: test.RandomBytes(32),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "unit 10 is not a non-fungible token type")
}

func TestUpdateNFT_InvalidBacklink(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: []byte{1},
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "invalid backlink")
}

func TestUpdateNFT_InvalidPredicateFormat(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:     test.RandomBytes(10),
			Backlink: make([]byte, 32),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "invalid script format")
}

func TestUpdateNFT_InvalidSignature(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:                test.RandomBytes(10),
			Backlink:            make([]byte, 32),
			DataUpdateSignature: script.PredicateAlwaysFalse(),
		}),
	)
	require.ErrorContains(t, txs.Execute(tx), "script execution result yielded false or non-clean stack")
}

func TestUpdateNFT_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := createNFTTypeAndMintToken(t, txs, nftTypeID, unitID)

	// update NFT
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Backlink:            make([]byte, 32),
			Data:                updatedData,
			DataUpdateSignature: script.PredicateArgumentEmpty(),
		}),
	)
	require.NoError(t, txs.Execute(tx))

	u, err := txs.state.GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, updatedData, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Nil(t, d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, script.PredicateAlwaysTrue(), []byte(u.Bearer))
}

func createNFTTypeAndMintToken(t *testing.T, txs *tokensTxSystem, nftTypeID []byte, nftID []byte) txsystem.GenericTransaction {
	// create NFT type
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(nftTypeID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
			DataUpdatePredicate:      script.PredicateAlwaysTrue(),
		}),
	)

	require.NoError(t, txs.Execute(tx))

	// mint NFT
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(nftID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                          script.PredicateAlwaysTrue(),
			NftType:                         nftTypeID,
			Uri:                             validNFTURI,
			Data:                            []byte{10},
			DataUpdatePredicate:             []byte{},
			TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
		}),
	)
	require.NoError(t, txs.Execute(tx))
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

func newTokenTxSystem(t *testing.T) *tokensTxSystem {
	txs, err := New()
	require.NoError(t, err)
	return txs
}

func signTx(t *testing.T, txs *tokensTxSystem, tx *txsystem.Transaction, signer crypto.Signer, pubKey []byte) ([]byte, []byte) {
	gtx, err := txs.ConvertTx(tx)
	require.NoError(t, err)

	signature, err := signer.SignBytes(gtx.SigBytes())
	require.NoError(t, err)

	gtx, err = txs.ConvertTx(tx)
	require.NoError(t, err)
	return signature, script.PredicateArgumentPayToPublicKeyHashDefault(signature, pubKey)
}
