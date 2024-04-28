package tokens

import (
	gocrypto "crypto"
	"fmt"
	"hash"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	hasherUtil "github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

const validNFTURI = "https://alphabill.org/nft"

var (
	parent1Identifier = tokens.NewNonFungibleTokenTypeID(nil, []byte{1})
	parent2Identifier = tokens.NewNonFungibleTokenTypeID(nil, []byte{2})
	nftTypeID1        = tokens.NewNonFungibleTokenTypeID(nil, []byte{10})
	nftTypeID2        = tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
	nftName           = fmt.Sprintf("Long name for %s", nftTypeID1)
)

var (
	nftUnitID                = tokens.NewNonFungibleTokenID(nil, []byte{1})
	symbol                   = "TEST"
	subTypeCreationPredicate = []byte{4}
	tokenCreationPredicate   = []byte{5}
	invariantPredicate       = []byte{6}
	dataUpdatePredicate      = []byte{7}
	updatedData              = []byte{0, 12}
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
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenCreationPredicate:   tokenCreationPredicate,
			InvariantPredicate:       invariantPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := txs.State().GetUnit(nftTypeID1, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenTypeData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenTypeData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, symbol, d.Symbol)
	require.Nil(t, d.ParentTypeId)
	require.Equal(t, subTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.Equal(t, tokenCreationPredicate, d.TokenCreationPredicate)
	require.Equal(t, invariantPredicate, d.InvariantPredicate)
	require.Equal(t, dataUpdatePredicate, d.DataUpdatePredicate)
}

func TestExecuteCreateNFTType_WithParentID(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	createParentTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent1Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(createParentTx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			&tokens.CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeID:                       parent1Identifier,
				SubTypeCreationPredicate:           templates.AlwaysFalseBytes(),
				SubTypeCreationPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
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

	txs, _ := newTokenTxSystem(t)

	// create parent1 type
	createParent1Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent1Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: parent1SubTypeCreationPredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(createParent1Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// create parent2 type
	unsignedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent2Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			&tokens.CreateNonFungibleTokenTypeAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
			},
		),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, p2pkhPredicateSig := signTx(t, unsignedCreateParent2Tx, parent2Signer, parent2PubKey)

	signedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent2Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			&tokens.CreateNonFungibleTokenTypeAttributes{
				Symbol:                             symbol,
				ParentTypeID:                       parent1Identifier,
				SubTypeCreationPredicate:           parent2SubTypeCreationPredicate,
				SubTypeCreationPredicateSignatures: [][]byte{p2pkhPredicateSig},
			},
		),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err = txs.Execute(signedCreateParent2Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// create child sub-type
	unsignedChildTxAttributes := &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		ParentTypeID:             parent2Identifier,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(), // no sub-types
	}
	createChildTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
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
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err = txs.Execute(createChildTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
}

func TestExecuteCreateNFTType_UnitIDIsNil(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nil),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_UnitIDHasWrongType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(tokens.NewNonFungibleTokenID(nil, test.RandomBytes(32))),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_ParentTypeIDHasWrongType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{ParentTypeID: tokens.NewNonFungibleTokenID(nil, test.RandomBytes(32))}),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidParentTypeID)
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_UnitIDExists(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol:                             symbol,
			ParentTypeID:                       parent1Identifier,
			SubTypeCreationPredicate:           subTypeCreationPredicate,
			SubTypeCreationPredicateSignatures: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", parent1Identifier))
	require.Nil(t, sm)
}

func TestExecuteCreateNFTType_InvalidParentType(t *testing.T) {
	txs, s := newTokenTxSystem(t)
	require.NoError(t, s.Apply(state.AddUnit(parent1Identifier, templates.AlwaysTrueBytes(), &mockUnitData{})))
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol:                             symbol,
			ParentTypeID:                       parent1Identifier,
			SubTypeCreationPredicate:           subTypeCreationPredicate,
			SubTypeCreationPredicateSignatures: [][]byte{{0}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.EqualError(t, err, fmt.Sprintf("tx order execution failed: invalid create non-fungible token type tx: token type SubTypeCreationPredicate: read [0] unit ID %q data: expected unit %[1]v data to be %T got %T", parent1Identifier, &tokens.NonFungibleTokenTypeData{}, &mockUnitData{}))
}

func TestExecuteCreateNFTType_InvalidSystemIdentifier(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(0),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{}),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid system identifier")
}

func TestExecuteCreateNFTType_InvalidTxType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{}),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "unknown transaction type")
}

func TestRevertTransaction_Ok(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{ParentTypeID: nil}),
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
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Name:   n,
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidNameLength)
}

func TestExecuteCreateNFTType_InvalidIconTypeLength(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Icon:   &tokens.Icon{Type: invalidIconType, Data: []byte{1, 2, 3}},
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidIconTypeLength)
}

func TestExecuteCreateNFTType_InvalidIconDataLength(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol: symbol,
			Icon:   &tokens.Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidIconDataLength)
}

func TestMintNFT_Ok(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
			Name:                             nftName,
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              templates.AlwaysTrueBytes(),
			TokenCreationPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)

	newTokenID := sm.TargetUnits[0]
	u, err := txs.State().GetUnit(newTokenID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())

	// verify unit log was added
	require.Len(t, u.Logs(), 1)

	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, []byte{10}, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, uint64(0), d.Counter)
}

func TestMintNFT_UnitIDIsNil(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nil),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
}

func TestMintNFT_UnitIDHasWrongType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(tokens.NewNonFungibleTokenID(nil, test.RandomBytes(32))),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidUnitID)
}

func TestMintNFT_UnitIDDoesNotExist(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              templates.AlwaysTrueBytes(),
			TokenCreationPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "not found")
}

func TestMintNFT_NFTTypeIsZero(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
			URI:                              validNFTURI,
			Data:                             []byte{10},
			DataUpdatePredicate:              templates.AlwaysTrueBytes(),
			TokenCreationPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "not found")
}

func TestMintNFT_NameLengthIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			Name: test.RandomString(maxNameLength + 1),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, ErrStrInvalidNameLength)
}

func TestMintNFT_URILengthIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			URI: test.RandomString(4097),
		}),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "URI exceeds the maximum allowed size of 4096 KB")
}

func TestMintNFT_URIFormatIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			URI: "invalid_uri",
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "URI invalid_uri is invalid")
}

func TestMintNFT_DataLengthIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
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
	txs, _ := newTokenTxSystem(t)

	typeID := tokens.NewNonFungibleTokenTypeID(nil, []byte{1})
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(typeID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			URI:                              validNFTURI,
			Data:                             []byte{0, 0, 0, 0},
			TokenCreationPredicateSignatures: [][]byte{{0}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", typeID))
}

func TestTransferNFT_UnitDoesNotExist(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	nonExistingUnitID := tokens.NewNonFungibleTokenID(nil, test.RandomBytes(32))
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nonExistingUnitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", nonExistingUnitID))
}

func TestTransferNFT_UnitIsNotNFT(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueBytes()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "invalid unit ID")
}

func TestTransferNFT_InvalidCounter(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      1,
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid counter")
}

func TestTransferNFT_InvalidTypeID(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    tokens.NewFungibleTokenTypeID(nil, test.RandomBytes(32)),
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid type identifier")
}

func TestTransferNFT_EmptyTypeID(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
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
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer NFT from 'always true' to 'p2pkh'
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    test.RandomBytes(32), // invalid bearer
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
			Nonce:                        test.RandomBytes(32),
			Counter:                      1,
			InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "tx order execution failed: invalid transfer non-fungible token tx: executing bearer predicate: decoding predicate:")
}

func TestTransferNFT_InvalidSignature(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer with invalid signature
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID: nftTypeID2,
			NewBearer: templates.AlwaysTrueBytes(),
			Nonce:     test.RandomBytes(32),
			Counter:   0,
			// the NFT we transfer has "always true" bearer predicate so providing
			// arguments for it makes it fail
			InvariantPredicateSignatures: [][]byte{{0x0B, 0x0A, 0x0D}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
		testtransaction.WithOwnerProof(test.RandomBytes(12)),
	)
	_, err := txs.Execute(tx)
	require.EqualError(t, err, `tx order execution failed: invalid transfer non-fungible token tx: executing bearer predicate: executing predicate: "always true" predicate arguments must be empty`)
}

func TestTransferNFT_Ok(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, []byte{10}, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Bearer()))
}

func TestTransferNFT_BurnedBearerMustFail(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer NFT, set bearer to un-spendable predicate
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysFalseBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	require.EqualValues(t, templates.AlwaysFalseBytes(), []byte(u.Bearer()))

	// the token must be considered as burned and not transferable
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysFalseBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      1,
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "executing bearer predicate: predicate evaluated to false")
}

func TestTransferNFT_LockedToken(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeLockToken),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.LockTokenAttributes{
			LockStatus:                   1,
			Counter:                      0,
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
	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	tokenData := u.Data().(*tokens.NonFungibleTokenData)
	require.EqualValues(t, 1, tokenData.Locked)

	// update nft
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.AlwaysTrueBytes(),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
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
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(dataMaxSize + 1),
			Counter: 0,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "data exceeds the maximum allowed size of 65536 KB")
}

func TestUpdateNFT_UnitDoesNotExist(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftUnitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(0),
			Counter: 0,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", nftUnitID))
}

func TestUpdateNFT_UnitIsNotNFT(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 0,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.ErrorContains(t, err, "invalid unit ID")
}

func TestUpdateNFT_LockedToken(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeLockToken),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.LockTokenAttributes{
			LockStatus:                   1,
			Counter:                      0,
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
	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	tokenData := u.Data().(*tokens.NonFungibleTokenData)
	require.EqualValues(t, 1, tokenData.Locked)

	// update nft
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 0,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)

	// verify token was locked
	require.ErrorContains(t, err, "token is locked")
}

func TestUpdateNFT_InvalidCounter(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 1,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.ErrorContains(t, err, "invalid counter")
}

func TestUpdateNFT_InvalidSignature(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	// create NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
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
	sm, err := txs.Execute(tx)
	require.NoError(t, err)

	nftID := sm.TargetUnits[0]

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 0,
			// the previous mint tx did set the DataUpdatePredicate to p2pkh so (for the tx
			// to be valid) the first argument here should be CBOR of pubkey and signature pair
			DataUpdateSignatures: [][]byte{{0}, templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(tx)
	require.EqualError(t, err, `tx order execution failed: invalid update non-fungible token tx: data update predicate: executing predicate: failed to decode P2PKH256 signature: cbor: cannot unmarshal positive integer into Go value of type templates.P2pkh256Signature`)
}

func TestUpdateNFT_Ok(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// update NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Counter:              0,
			Data:                 updatedData,
			DataUpdateSignatures: [][]byte{nil, nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err := txs.Execute(tx)
	require.NoError(t, err)
	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, updatedData, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Bearer()))
}

// Test LockFC -> UnlockFC
func TestExecute_LockFeeCreditTxs_OK(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	err := txs.BeginBlock(1)
	require.NoError(t, err)

	// lock fee credit record
	lockFCAttr := testutils.NewLockFCAttr(testutils.WithLockFCBacklink(make([]byte, 32)))
	lockFC := testutils.NewLockFC(t, lockFCAttr,
		testtransaction.WithUnitID(feeCreditID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
	)
	sm, err := txs.Execute(lockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify unit was locked
	u, err := txs.State().GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.True(t, fcr.IsLocked())

	// unlock fee credit record
	unlockFCAttr := testutils.NewUnlockFCAttr(testutils.WithUnlockFCBacklink(lockFC.Hash(gocrypto.SHA256)))
	unlockFC := testutils.NewUnlockFC(t, unlockFCAttr,
		testtransaction.WithUnitID(feeCreditID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
	)
	sm, err = txs.Execute(unlockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify unit was unlocked
	fcrUnit, err := txs.State().GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcr, ok = fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.False(t, fcr.IsLocked())
}

func createNFTTypeAndMintToken(t *testing.T, txs *txsystem.GenericTxSystem, nftTypeID types.UnitID) types.UnitID {
	// create NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeCreateNFTType),
		testtransaction.WithUnitID(nftTypeID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.CreateNonFungibleTokenTypeAttributes{
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

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(nftTypeID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			Bearer:                           templates.AlwaysTrueBytes(),
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
	sm, err := txs.Execute(tx)
	require.NoError(t, err)

	return sm.TargetUnits[0]
}

type mockUnitData struct{}

func (m mockUnitData) Write(hash.Hash) error { return nil }

func (m mockUnitData) SummaryValueInput() uint64 {
	return 0
}

func (m mockUnitData) Copy() types.UnitData {
	return &mockUnitData{}
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

func newTokenTxSystem(t *testing.T) (*txsystem.GenericTxSystem, *state.State) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(state.AddUnit(feeCreditID, templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{
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
		observability.Default(t),
		WithTrustBase(map[string]abcrypto.Verifier{"test": verifier}),
		WithState(s),
	)
	require.NoError(t, err)
	return txs, s
}
