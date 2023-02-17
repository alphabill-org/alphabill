package tokens

import (
	gocrypto "crypto"
	"fmt"
	"hash"
	"math/rand"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/crypto"
	hasher "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
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
	_, verifier := testsig.CreateSignerAndVerifier(t)
	systemIdentifier := []byte{0, 0, 0, 7}
	txs, err := New(
		WithSystemIdentifier(systemIdentifier),
		WithHashAlgorithm(gocrypto.SHA512),
		WithTrustBase(map[string]crypto.Verifier{"test2": verifier}),
	)
	require.NoError(t, err)
	require.Equal(t, gocrypto.SHA512, txs.HashAlgorithm())
	require.Equal(t, systemIdentifier, txs.SystemIdentifier())

	require.NotNil(t, txs.GetState())
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	require.NoError(t, txs.Execute(tx))
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
	createParentTx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent1Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	require.NoError(t, txs.Execute(createParentTx))
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeId:                       util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
				SubTypeCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
	parent2SubTypeCreationPredicate[0] = script.StartByte // verify parent1SubTypeCreationPredicate signature verification result

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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	signature, p2pkhPredicate := signTx(t, txs, unsignedCreateParent2Tx, parent2Signer, parent2PubKey)

	signedCreateParent2Tx := testtransaction.NewTransaction(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(parent2Identifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeId:                       util.Uint256ToBytes(parent1Identifier),
				SubTypeCreationPredicate:           parent2SubTypeCreationPredicate,
				SubTypeCreationPredicateSignatures: [][]byte{p2pkhPredicate},
			},
		),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	gtx, err = txs.ConvertTx(createChildTx)
	require.NoError(t, err)

	signature, err = childSigner.SignBytes(gtx.SigBytes())
	require.NoError(t, err)
	signature2, err := parent2Signer.SignBytes(gtx.SigBytes())
	require.NoError(t, err)

	// child owner proof must satisfy parent1 & parent2 SubTypeCreationPredicates
	unsignedChildTxAttributes.SubTypeCreationPredicateSignatures = [][]byte{
		script.PredicateArgumentPayToPublicKeyHashDefault(signature, childPublicKey), // parent2 p2pkhPredicate argument
		script.PredicateArgumentPayToPublicKeyHashDefault(signature2, parent2PubKey), // parent1 p2pkhPredicate argument
	}
	createChildTx = testtransaction.NewTransaction(
		t,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.ErrorContains(t, txs.Execute(tx), fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(parent1Identifier)))
}

func TestExecuteCreateNFTType_InvalidParentType(t *testing.T) {
	txs := newTokenTxSystem(t)
	txs.GetState().AtomicUpdate(rma.AddItem(parent1Identifier, script.PredicateAlwaysTrue(), &mockUnitData{}, []byte{}))
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
	txs := newTokenTxSystem(t)
	tx := moneytesttx.RandomGenericBillTransfer(t)
	tx.ToProtoBuf().SystemId = DefaultTokenTxSystemIdentifier
	tx.ToProtoBuf().ClientMetadata = &txsystem.ClientMetadata{
		Timeout:           1000,
		MaxFee:            10,
		FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
	}
	tx.ToProtoBuf().FeeProof = script.PredicateArgumentEmpty()
	require.ErrorContains(t, txs.Execute(tx), "unknown transaction type")
}

func TestRevertTransaction_Ok(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.NoError(t, txs.Execute(tx))
	txs.Revert()
	_, err := txs.GetState().GetUnit(unitIdentifier)
	require.ErrorContains(t, err, fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(unitIdentifier)))
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	require.NoError(t, txs.Execute(tx))
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NftType:                          nftTypeID,
			Uri:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.NoError(t, txs.Execute(tx))
	u, err := txs.GetState().GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	txHash := tx.Hash(gocrypto.SHA256)
	require.Equal(t, txHash, u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, []byte{10}, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Equal(t, script.PredicateAlwaysTrue(), d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, txHash, d.backlink)
}

func TestMintNFT_UnitIDIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(uint256.NewInt(0))),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	require.NoError(t, txs.Execute(tx))
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NftType:                          nftTypeID,
			Uri:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.NoError(t, txs.Execute(tx))
	require.ErrorContains(t, txs.Execute(tx), "unit 1 exist")
}

func TestMintNFT_NFTTypeIsZero(t *testing.T) {
	txs := newTokenTxSystem(t)
	idBytes := util.Uint256ToBytes(uint256.NewInt(110))
	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NftType:                          idBytes,
			Uri:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.ErrorContains(t, txs.Execute(tx), fmt.Sprintf("item %X does not exist", idBytes))
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.ErrorContains(t, txs.Execute(tx), "item 0000000000000000000000000000000000000000000000000000000000000001 does not exist")
}

func TestTransferNFT_UnitDoesNotExist(t *testing.T) {
	txs := newTokenTxSystem(t)

	tx := testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     test.RandomBytes(32),
			InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysTrue()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.ErrorContains(t, txs.Execute(tx), "item 0000000000000000000000000000000000000000000000000000000000000001 does not exist")
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.NoError(t, txs.Execute(tx))

	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(util.Uint256ToBytes(unitIdentifier)),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     test.RandomBytes(32),
			InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysTrue()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     []byte{1},
			InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
			NewBearer:                    script.PredicateAlwaysTrue(),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     tx.Hash(gocrypto.SHA256),
			InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.NoError(t, txs.Execute(tx))

	u, err := txs.GetState().GetUnit(uint256.NewInt(0).SetBytes(unitID))
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.ErrorContains(t, txs.Execute(tx), "item 0000000000000000000000000000000000000000000000000000000000000001 does not exist")
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
			Data:                 test.RandomBytes(10),
			Backlink:             tx.Hash(gocrypto.SHA256),
			DataUpdateSignatures: [][]byte{script.PredicateArgumentEmpty(), {}},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
			Data:                 test.RandomBytes(10),
			Backlink:             tx.Hash(gocrypto.SHA256),
			DataUpdateSignatures: [][]byte{script.PredicateAlwaysTrue(), script.PredicateAlwaysFalse()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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
			Backlink:             tx.Hash(gocrypto.SHA256),
			Data:                 updatedData,
			DataUpdateSignatures: [][]byte{script.PredicateArgumentEmpty(), script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)
	require.NoError(t, txs.Execute(tx))

	u, err := txs.GetState().GetUnit(uint256.NewInt(0).SetBytes(unitID))
	require.NoError(t, err)
	require.Equal(t, tx.Hash(gocrypto.SHA256), u.StateHash)
	require.IsType(t, &nonFungibleTokenData{}, u.Data)
	d := u.Data.(*nonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.Value())
	require.Equal(t, uint256.NewInt(0).SetBytes(nftTypeID), d.nftType)
	require.Equal(t, updatedData, d.data)
	require.Equal(t, validNFTURI, d.uri)
	require.Equal(t, script.PredicateAlwaysTrue(), d.dataUpdatePredicate)
	require.Equal(t, uint64(0), d.t)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, script.PredicateAlwaysTrue(), []byte(u.Bearer))
}

func createNFTTypeAndMintToken(t *testing.T, txs *txsystem.ModularTxSystem, nftTypeID []byte, nftID []byte) txsystem.GenericTransaction {
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
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
	)

	require.NoError(t, txs.Execute(tx))

	// mint NFT
	tx = testtransaction.NewGenericTransaction(
		t,
		txs.ConvertTx,
		testtransaction.WithUnitId(nftID),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           script.PredicateAlwaysTrue(),
			NftType:                          nftTypeID,
			Uri:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              script.PredicateAlwaysTrue(),
			TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
		}),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout:           1000,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
		}),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
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

func signTx(t *testing.T, txs *txsystem.ModularTxSystem, tx *txsystem.Transaction, signer crypto.Signer, pubKey []byte) ([]byte, []byte) {
	gtx, err := txs.ConvertTx(tx)
	require.NoError(t, err)

	signature, err := signer.SignBytes(gtx.SigBytes())
	require.NoError(t, err)

	gtx, err = txs.ConvertTx(tx)
	require.NoError(t, err)
	return signature, script.PredicateArgumentPayToPublicKeyHashDefault(signature, pubKey)
}

func newTokenTxSystem(t *testing.T) *txsystem.ModularTxSystem {
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
