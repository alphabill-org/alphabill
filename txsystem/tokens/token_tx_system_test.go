package tokens

import (
	"fmt"
	"hash"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	hasherUtil "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

const validNFTURI = "https://alphabill.org/nft"

var (
	parent1Identifier         = tokens.NewNonFungibleTokenTypeID(nil, []byte{1})
	parent2Identifier         = tokens.NewNonFungibleTokenTypeID(nil, []byte{2})
	nftTypeID1                = tokens.NewNonFungibleTokenTypeID(nil, []byte{10})
	nftTypeID2                = tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
	nftName                   = fmt.Sprintf("Long name for %s", nftTypeID1)
	invalidNonFungibleTokenID = tokens.NewFungibleTokenID(nil, []byte{1}) // use fungible type id
)

var (
	nftUnitID                = tokens.NewNonFungibleTokenID(nil, []byte{1})
	symbol                   = "TEST"
	subTypeCreationPredicate = []byte{4}
	tokenMintingPredicate    = []byte{5}
	tokenTypeOwnerPredicate  = []byte{6}
	dataUpdatePredicate      = []byte{7}
	updatedData              = []byte{0, 12}
)

func TestNewTokenTxSystem_NilSystemIdentifier(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		SystemIdentifier: 0,
		TypeIdLen:        8,
		UnitIdLen:        256,
		T2Timeout:        2000 * time.Millisecond,
	}
	txs, err := NewTxSystem(pdr, types.ShardID{}, nil, WithState(state.NewEmptyState()))
	require.ErrorContains(t, err, `system identifier is missing`)
	require.Nil(t, txs)
}

func TestNewTokenTxSystem_StateIsNil(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		SystemIdentifier: tokens.DefaultSystemID,
		TypeIdLen:        8,
		UnitIdLen:        256,
		T2Timeout:        2000 * time.Millisecond,
	}
	txs, err := NewTxSystem(pdr, types.ShardID{}, nil, WithState(nil))
	require.ErrorContains(t, err, ErrStrStateIsNil)
	require.Nil(t, txs)
}

func TestExecuteDefineNFT_WithoutParentID(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenMintingPredicate:    tokenMintingPredicate,
			TokenTypeOwnerPredicate:  tokenTypeOwnerPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	u, err := txs.State().GetUnit(nftTypeID1, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenTypeData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenTypeData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, symbol, d.Symbol)
	require.Nil(t, d.ParentTypeID)
	require.Equal(t, subTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.Equal(t, tokenMintingPredicate, d.TokenMintingPredicate)
	require.Equal(t, tokenTypeOwnerPredicate, d.TokenTypeOwnerPredicate)
	require.Equal(t, dataUpdatePredicate, d.DataUpdatePredicate)
}

func TestExecuteDefineNFT_WithParentID(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	createParentTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent1Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(createParentTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{createParentTx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			&tokens.DefineNonFungibleTokenAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
			},
		),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{nil}}),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
}

func TestExecuteDefineNFT_InheritanceChainWithP2PKHPredicates(t *testing.T) {
	// Inheritance Chain: parent1Identifier <- parent2Identifier <- unitIdentifier
	parent2Signer, parent2PubKey := createSigner(t)
	childSigner, childPublicKey := createSigner(t)

	// only parent2 can create subtypes from parent1
	parent1SubTypeCreationPredicate := templates.NewP2pkh256BytesFromKeyHash(hasherUtil.Sum256(parent2PubKey))

	// parent2 and child together can create a subtype because SubTypeCreationPredicate are concatenated (ownerProof must contain both signatures)
	parent2SubTypeCreationPredicate := templates.NewP2pkh256BytesFromKeyHash(hasherUtil.Sum256(childPublicKey))

	txs, _ := newTokenTxSystem(t)

	// create parent1 type
	createParent1Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent1Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: parent1SubTypeCreationPredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(createParent1Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{createParent1Tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	// create parent2 type
	unsignedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent2Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			&tokens.DefineNonFungibleTokenAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
			},
		),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, p2pkhPredicateSig := signTx(t, unsignedCreateParent2Tx, parent2Signer, parent2PubKey)

	signedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent2Identifier),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			&tokens.DefineNonFungibleTokenAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
				//SubTypeCreationProofs: [][]byte{p2pkhPredicateSig},
			},
		),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{p2pkhPredicateSig}}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err = txs.Execute(signedCreateParent2Tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{signedCreateParent2Tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	// create child subtype
	unsignedChildTxAttributes := &tokens.DefineNonFungibleTokenAttributes{
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
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithFeeProof(nil),
	)

	sigBytes, err := createChildTx.PayloadBytes()
	require.NoError(t, err)

	signature, err := childSigner.SignBytes(sigBytes)
	require.NoError(t, err)
	signature2, err := parent2Signer.SignBytes(sigBytes)
	require.NoError(t, err)

	createChildTx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{
			SubTypeCreationProofs: [][]byte{
				templates.NewP2pkh256SignatureBytes(signature, childPublicKey), // parent2 p2pkhPredicate argument
				templates.NewP2pkh256SignatureBytes(signature2, parent2PubKey), // parent1 p2pkhPredicate argument
			},
		}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err = txs.Execute(createChildTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{createChildTx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
}

func TestExecuteDefineNFT_UnitIDIsNil(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nil),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.EqualError(t, err, `invalid transaction: expected 33 byte unit ID, got 0 bytes`)
	require.Nil(t, sm)
}

func TestExecuteDefineNFT_UnitIDHasWrongType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(tokens.NewNonFungibleTokenID(nil, test.RandomBytes(32))),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidUnitID)
}

func TestExecuteDefineNFT_ParentTypeIDHasWrongType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{ParentTypeID: tokens.NewNonFungibleTokenID(nil, test.RandomBytes(32))}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidParentTypeID)
}

func TestExecuteDefineNFT_UnitIDExists(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)

	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), fmt.Sprintf("unit %s exists", nftTypeID1))
}

func TestExecuteDefineNFT_ParentDoesNotExist(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			ParentTypeID:             parent1Identifier,
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{templates.EmptyArgument()}}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{templates.EmptyArgument()}}),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), fmt.Sprintf("item %s does not exist", parent1Identifier))
}

func TestExecuteDefineNFT_InvalidParentType(t *testing.T) {
	txs, s := newTokenTxSystem(t)
	require.NoError(t, s.Apply(state.AddUnit(parent1Identifier, templates.AlwaysTrueBytes(), &mockUnitData{})))
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			ParentTypeID:             parent1Identifier,
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{{0}}}),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.EqualError(t, sm.ErrDetail(), fmt.Sprintf("transaction 'defNT' validation error: token type SubTypeCreationPredicate: read [0] unit ID %q data: expected unit %[1]v data to be %T got %T", parent1Identifier, &tokens.NonFungibleTokenTypeData{}, &mockUnitData{}))
}

func TestExecuteDefineNFT_InvalidSystemIdentifier(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(0),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
	)
	sm, err := txs.Execute(tx)
	require.EqualError(t, err, "invalid transaction: error invalid system identifier")
	require.Nil(t, sm)
}

func TestExecuteDefineNFT_InvalidTxType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), "unknown transaction type")
}

func TestRevertTransaction_Ok(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{ParentTypeID: nil}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.NoError(t, sm.ErrDetail())
	txs.Revert()

	_, err = txs.State().GetUnit(nftTypeID1, false)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", nftTypeID1))
}

func TestExecuteDefineNFT_InvalidSymbolLength(t *testing.T) {
	s := "♥ Alphabill ♥"
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{Symbol: s}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidSymbolLength)
}

func TestExecuteDefineNFT_InvalidNameLength(t *testing.T) {
	n := "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill♥♥"
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol: symbol,
			Name:   n,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidNameLength)
}

func TestExecuteDefineNFT_InvalidIconTypeLength(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol: symbol,
			Icon:   &tokens.Icon{Type: invalidIconType, Data: []byte{1, 2, 3}},
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidIconTypeLength)
}

func TestExecuteDefineNFT_InvalidIconDataLength(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol: symbol,
			Icon:   &tokens.Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidIconDataLength)
}

func TestMintNFT_Ok(t *testing.T) {
	mintingSigner, mintingVerifier := testsig.CreateSignerAndVerifier(t)
	mintingPublicKey, err := mintingVerifier.MarshalPublicKey()
	require.NoError(t, err)

	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenMintingPredicate:    templates.NewP2pkh256BytesFromKey(mintingPublicKey),
			TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			OwnerPredicate:      templates.AlwaysTrueBytes(),
			TypeID:              nftTypeID2,
			Name:                nftName,
			URI:                 validNFTURI,
			Data:                []byte{10},
			DataUpdatePredicate: templates.AlwaysTrueBytes(),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	newTokenID := newNonFungibleTokenID(t, tx)
	tx.Payload.UnitID = newTokenID

	// set minting predicate
	ownerProof := testsig.NewOwnerProof(t, tx, mintingSigner)
	authProof := tokens.MintNonFungibleTokenAuthProof{TokenMintingProof: ownerProof}
	require.NoError(t, tx.SetAuthProof(authProof))

	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)

	u, err := txs.State().GetUnit(newTokenID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())

	// verify unit log was added
	require.Len(t, u.Logs(), 1)

	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.TypeID)
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
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.EqualError(t, err, `invalid transaction: expected 33 byte unit ID, got 0 bytes`)
	require.Nil(t, sm)
}

func TestMintNFT_UnitIDHasWrongType(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithUnitID(invalidNonFungibleTokenID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidUnitID)
}

func TestMintNFT_AlreadyExists(t *testing.T) {
	txs, s := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenMintingPredicate:    templates.AlwaysTrueBytes(),
			TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.NoError(t, sm.ErrDetail())

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			OwnerPredicate:      templates.AlwaysTrueBytes(),
			TypeID:              nftTypeID2,
			URI:                 validNFTURI,
			Data:                []byte{10},
			DataUpdatePredicate: templates.AlwaysTrueBytes(),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{TokenMintingProof: templates.EmptyArgument()}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	tokenID := newNonFungibleTokenID(t, tx)
	tx.Payload.UnitID = tokenID

	err = s.Apply(state.AddUnit(tokenID, templates.AlwaysTrueBytes(), tokens.NewNonFungibleTokenData(nftTypeID2, &tokens.MintNonFungibleTokenAttributes{}, 0, 0)))
	require.NoError(t, err)

	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "token already exists")
}

func TestMintNFT_NameLengthIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			Name:   test.RandomString(maxNameLength + 1),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	tx.Payload.UnitID = newNonFungibleTokenID(t, tx)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), ErrStrInvalidNameLength)
}

func TestMintNFT_URILengthIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			URI:    test.RandomString(4097),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	tx.Payload.UnitID = newNonFungibleTokenID(t, tx)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "URI exceeds the maximum allowed size of 4096 KB")
}

func TestMintNFT_URIFormatIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			URI:    "invalid_uri",
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	tx.Payload.UnitID = newNonFungibleTokenID(t, tx)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "URI invalid_uri is invalid")
}

func TestMintNFT_DataLengthIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			URI:    validNFTURI,
			Data:   test.RandomBytes(dataMaxSize + 1),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	tx.Payload.UnitID = newNonFungibleTokenID(t, tx)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "data exceeds the maximum allowed size of 65536 KB")
}

func TestMintNFT_NFTTypeDoesNotExist(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	typeID := tokens.NewNonFungibleTokenTypeID(nil, []byte{1})
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			URI:    validNFTURI,
			Data:   []byte{0, 0, 0, 0},
			TypeID: typeID,
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{
			TokenMintingProof: []byte{0}},
		),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	tx.Payload.UnitID = newNonFungibleTokenID(t, tx)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "nft type does not exist")
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
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: templates.AlwaysTrueBytes(),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), fmt.Sprintf("item %s does not exist", nonExistingUnitID))
}

func TestTransferNFT_UnitIsNotNFT(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenMintingPredicate:    tokenMintingPredicate,
			TokenTypeOwnerPredicate:  tokenTypeOwnerPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: templates.AlwaysTrueBytes(),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "invalid unit ID")
}

func TestTransferNFT_InvalidCounter(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           1,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{OwnerProof: templates.EmptyArgument()}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "invalid counter")
}

func TestTransferNFT_InvalidTypeID(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            tokens.NewFungibleTokenTypeID(nil, test.RandomBytes(32)),
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: []byte{0, 0, 0, 1},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "invalid type identifier")
}

func TestTransferNFT_EmptyTypeID(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			TokenTypeOwnerProofs: [][]byte{{0, 0, 0, 1}},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
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
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer NFT from 'always true' to 'p2pkh'
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            nftTypeID2,
			NewOwnerPredicate: test.RandomBytes(32), // invalid owner
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            nftTypeID2,
			NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
			Counter:           1,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof:           templates.EmptyArgument(),
			TokenTypeOwnerProofs: nil,
		}),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "transaction 'transNT' validation error: evaluating owner predicate: decoding predicate:")
}

func TestTransferNFT_InvalidSignature(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer with invalid signature
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),

		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            nftTypeID2,
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: test.RandomBytes(12),
			// the NFT we transfer has "always true" bearer predicate so providing
			// arguments for it makes it fail
			TokenTypeOwnerProofs: [][]byte{{0x0B, 0x0A, 0x0D}},
		}),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.EqualError(t, sm.ErrDetail(), `transaction 'transNT' validation error: evaluating owner predicate: executing predicate: "always true" predicate arguments must be empty`)
}

func TestTransferNFT_Ok(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            nftTypeID2,
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.TypeID)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, []byte{10}, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Owner()))
}

func TestTransferNFT_BurnedBearerMustFail(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer NFT, set bearer to un-spendable predicate
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            nftTypeID2,
			NewOwnerPredicate: templates.AlwaysFalseBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	require.EqualValues(t, templates.AlwaysFalseBytes(), []byte(u.Owner()))

	// the token must be considered as burned and not transferable
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),

		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            nftTypeID2,
			NewOwnerPredicate: templates.AlwaysFalseBytes(),
			Counter:           1,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "evaluating owner predicate: predicate evaluated to \"false\"")
}

func TestTransferNFT_LockedToken(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeLockToken),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.LockTokenAttributes{
			LockStatus: 1,
			Counter:    0,
		}),
		testtransaction.WithAuthProof(&tokens.LockTokenAuthProof{
			OwnerProof: templates.EmptyArgument(),
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
	)
	sm, err := txs.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

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
			TypeID:            nftTypeID2,
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: templates.EmptyArgument(),
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "token is locked")
}

func TestUpdateNFT_DataLengthIsInvalid(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(dataMaxSize + 1),
			Counter: 0,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "data exceeds the maximum allowed size of 65536 KB")
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
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), fmt.Sprintf("item %s does not exist", nftUnitID))
}

func TestUpdateNFT_UnitIsNotNFT(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenMintingPredicate:    tokenMintingPredicate,
			TokenTypeOwnerPredicate:  tokenTypeOwnerPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 0,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "invalid unit ID")
}

func TestUpdateNFT_LockedToken(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeLockToken),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.LockTokenAttributes{
			LockStatus: 1,
			Counter:    0,
		}),
		testtransaction.WithAuthProof(&tokens.LockTokenAuthProof{OwnerProof: templates.EmptyArgument()}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
	)
	sm, err := txs.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{lockTx.UnitID(), feeCreditID}, sm.TargetUnits)

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
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.ErrorContains(t, sm.ErrDetail(), "token is locked")
}

func TestUpdateNFT_InvalidCounter(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 1,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.ErrorContains(t, sm.ErrDetail(), "invalid counter")
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
}

func TestUpdateNFT_InvalidSignature(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	// create NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenMintingPredicate:    templates.AlwaysTrueBytes(),
			TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithFeeProof(nil),
	)

	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.NoError(t, sm.ErrDetail())

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			OwnerPredicate:      templates.AlwaysTrueBytes(),
			TypeID:              nftTypeID2,
			Name:                nftName,
			URI:                 validNFTURI,
			Data:                []byte{10},
			DataUpdatePredicate: templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{
			TokenMintingProof: templates.EmptyArgument()},
		),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithFeeProof(nil),
	)
	nftID := newNonFungibleTokenID(t, tx)
	tx.Payload.UnitID = nftID

	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.NoError(t, sm.ErrDetail())
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 0,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{
			// the previous mint tx did set the DataUpdatePredicate to p2pkh so for the tx to be valid
			// the first argument here should be CBOR of pubkey and signature pair
			TokenDataUpdateProof:      []byte{0},
			TokenTypeDataUpdateProofs: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, sm.TargetUnits)
	require.EqualError(t, sm.ErrDetail(), `transaction 'updateNT' validation error: data update predicate: executing predicate: failed to decode P2PKH256 signature: cbor: cannot unmarshal positive integer into Go value of type templates.P2pkh256Signature`)
}

func TestUpdateNFT_Ok(t *testing.T) {
	txs, _ := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// update NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Counter: 0,
			Data:    updatedData,
			//DataUpdateSignatures: [][]byte{nil, nil},
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{
			TokenDataUpdateProof:      templates.EmptyArgument(),
			TokenTypeDataUpdateProofs: [][]byte{templates.EmptyArgument()},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{nftID, feeCreditID}, sm.TargetUnits)

	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.TypeID)
	require.Equal(t, nftName, d.Name)
	require.Equal(t, updatedData, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Owner()))
}

// Test LockFC -> UnlockFC
func TestExecute_LockFeeCreditTxs_OK(t *testing.T) {
	txs, _ := newTokenTxSystem(t)

	err := txs.BeginBlock(1)
	require.NoError(t, err)

	// lock fee credit record
	signer, _ := testsig.CreateSignerAndVerifier(t)
	lockFCAttr := testutils.NewLockFCAttr(testutils.WithLockFCCounter(10))
	lockFC := testutils.NewLockFC(t, signer, lockFCAttr,
		testtransaction.WithUnitID(feeCreditID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
	)
	sm, err := txs.Execute(lockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{lockFC.UnitID()}, sm.TargetUnits)

	// verify unit was locked
	u, err := txs.State().GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.True(t, fcr.IsLocked())

	// unlock fee credit record
	unlockFCAttr := testutils.NewUnlockFCAttr(testutils.WithUnlockFCCounter(11))
	unlockFC := testutils.NewUnlockFC(t, signer, unlockFCAttr,
		testtransaction.WithUnitID(feeCreditID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
	)
	sm, err = txs.Execute(unlockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{unlockFC.UnitID()}, sm.TargetUnits)

	// verify unit was unlocked
	fcrUnit, err := txs.State().GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcr, ok = fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.False(t, fcr.IsLocked())
}

func defineNFTAndMintToken(t *testing.T, txs *txsystem.GenericTxSystem, nftTypeID types.UnitID) types.UnitID {
	// define NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenMintingPredicate:    templates.AlwaysTrueBytes(),
			TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithFeeProof(nil),
	)

	sm, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeMintNFT),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			OwnerPredicate:      templates.AlwaysTrueBytes(),
			TypeID:              nftTypeID,
			Name:                nftName,
			URI:                 validNFTURI,
			Data:                []byte{10},
			DataUpdatePredicate: templates.AlwaysTrueBytes(),
		}),
		testtransaction.WithAuthProof(tokens.MintNonFungibleTokenAuthProof{TokenMintingProof: templates.EmptyArgument()}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithFeeProof(nil),
	)
	tx.Payload.UnitID = newNonFungibleTokenID(t, tx)
	sm, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID(), feeCreditID}, sm.TargetUnits)
	require.True(t, sm.ActualFee > 0)
	return tx.Payload.UnitID
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
		Balance: 100,
		Counter: 10,
		Timeout: 1000,
	})))
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  1,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}))
	pdr := types.PartitionDescriptionRecord{
		SystemIdentifier: tokens.DefaultSystemID,
		TypeIdLen:        8,
		UnitIdLen:        256,
		T2Timeout:        2000 * time.Millisecond,
	}

	txs, err := NewTxSystem(
		pdr,
		types.ShardID{},
		observability.Default(t),
		WithTrustBase(testtb.NewTrustBase(t, verifier)),
		WithState(s),
	)
	require.NoError(t, err)
	return txs, s
}
