package tokens

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
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
	parent1Identifier         types.UnitID = append(make(types.UnitID, 31), 1, tokens.NonFungibleTokenTypeUnitType)
	parent2Identifier         types.UnitID = append(make(types.UnitID, 31), 2, tokens.NonFungibleTokenTypeUnitType)
	nftTypeID1                types.UnitID = append(make(types.UnitID, 31), 10, tokens.NonFungibleTokenTypeUnitType)
	nftTypeID2                types.UnitID = append(make(types.UnitID, 31), 11, tokens.NonFungibleTokenTypeUnitType)
	invalidNonFungibleTokenID types.UnitID = append(make(types.UnitID, 31), 1, tokens.FungibleTokenUnitType) // use fungible type id
	nftName                                = fmt.Sprintf("Long name for %s", nftTypeID1)
)

var (
	symbol                   = "TEST"
	subTypeCreationPredicate = []byte{4}
	tokenMintingPredicate    = []byte{5}
	tokenTypeOwnerPredicate  = []byte{6}
	dataUpdatePredicate      = []byte{7}
	updatedData              = []byte{0, 12}
)

func TestNewTokenTxSystem_NilPartitionID(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: 0,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2000 * time.Millisecond,
	}
	txs, err := NewTxSystem(pdr, types.ShardID{}, nil, WithState(state.NewEmptyState()))
	require.ErrorContains(t, err, `failed to load permissionless fee credit module: invalid fee credit module configuration: invalid PDR: invalid partition identifier: 00000000`)
	require.Nil(t, txs)
}

func TestNewTokenTxSystem_StateIsNil(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: tokens.DefaultPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2000 * time.Millisecond,
	}
	txs, err := NewTxSystem(pdr, types.ShardID{}, nil, WithState(nil))
	require.ErrorContains(t, err, ErrStrStateIsNil)
	require.Nil(t, txs)
}

func TestExecuteDefineNFT_WithoutParentID(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			TokenMintingPredicate:    tokenMintingPredicate,
			TokenTypeOwnerPredicate:  tokenTypeOwnerPredicate,
			DataUpdatePredicate:      dataUpdatePredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	//require.NoError(t,tokens.UnitIDGenerator(pdr)(tx,types.ShardID{}))

	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	u, err := txs.State().GetUnit(nftTypeID1, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenTypeData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenTypeData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, symbol, d.Symbol)
	require.Nil(t, d.ParentTypeID)
	require.EqualValues(t, subTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.EqualValues(t, tokenMintingPredicate, d.TokenMintingPredicate)
	require.EqualValues(t, tokenTypeOwnerPredicate, d.TokenTypeOwnerPredicate)
	require.EqualValues(t, dataUpdatePredicate, d.DataUpdatePredicate)
}

func TestExecuteDefineNFT_WithParentID(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	createParentTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent1Identifier),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(createParentTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{createParentTx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(
			&tokens.DefineNonFungibleTokenAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
			},
		),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{nil}}),
	)
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
}

func TestExecuteDefineNFT_InheritanceChainWithP2PKHPredicates(t *testing.T) {
	// Inheritance Chain: parent1Identifier <- parent2Identifier <- unitIdentifier
	parent2Signer, parent2PubKey := createSigner(t)
	childSigner, childPublicKey := createSigner(t)

	// only parent2 can create subtypes from parent1
	parent1SubTypeCreationPredicate := templates.NewP2pkh256BytesFromKeyHash(abhash.Sum256(parent2PubKey))

	// parent2 and child together can create a subtype because SubTypeCreationPredicate are concatenated (ownerProof must contain both signatures)
	parent2SubTypeCreationPredicate := templates.NewP2pkh256BytesFromKeyHash(abhash.Sum256(childPublicKey))

	txs, _, _ := newTokenTxSystem(t)

	// create parent1 type
	createParent1Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent1Identifier),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: parent1SubTypeCreationPredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(createParent1Tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{createParent1Tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	// create parent2 type
	unsignedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent2Identifier),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(
			&tokens.DefineNonFungibleTokenAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
			},
		),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, p2pkhPredicateSig := signTx(t, unsignedCreateParent2Tx, parent2Signer, parent2PubKey)

	signedCreateParent2Tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(parent2Identifier),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(
			&tokens.DefineNonFungibleTokenAttributes{
				Symbol:                   symbol,
				ParentTypeID:             parent1Identifier,
				SubTypeCreationPredicate: parent2SubTypeCreationPredicate,
				//SubTypeCreationProofs: [][]byte{p2pkhPredicateSig},
			},
		),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{p2pkhPredicateSig}}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	txr, err = txs.Execute(signedCreateParent2Tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{signedCreateParent2Tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	// create child subtype
	unsignedChildTxAttributes := &tokens.DefineNonFungibleTokenAttributes{
		Symbol:                   symbol,
		ParentTypeID:             parent2Identifier,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(), // no sub-types
	}
	createChildTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithFeeProof(nil),
	)

	sigBytes, err := createChildTx.AuthProofSigBytes()
	require.NoError(t, err)

	signature, err := childSigner.SignBytes(sigBytes)
	require.NoError(t, err)
	signature2, err := parent2Signer.SignBytes(sigBytes)
	require.NoError(t, err)

	createChildTx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(
			unsignedChildTxAttributes,
		),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{
			SubTypeCreationProofs: [][]byte{
				templates.NewP2pkh256SignatureBytes(signature, childPublicKey), // parent2 p2pkhPredicate argument
				templates.NewP2pkh256SignatureBytes(signature2, parent2PubKey), // parent1 p2pkhPredicate argument
			},
		}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)

	txr, err = txs.Execute(createChildTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{createChildTx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
}

func TestExecuteDefineNFT_UnitIDIsNil(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nil),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.EqualError(t, err, `invalid transaction: expected 33 byte unit ID, got 0 bytes`)
	require.Nil(t, txr)
}

func TestExecuteDefineNFT_UnitIDHasWrongType(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(tokenid.NewNonFungibleTokenID(t)),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), `transaction validation error (type=2): invalid nft ID: expected type 0X2, got 0X4`)
}

func TestExecuteDefineNFT_ParentTypeIDHasWrongType(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{ParentTypeID: tokenid.NewNonFungibleTokenID(t)}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), ErrStrInvalidParentTypeID)
}

func TestExecuteDefineNFT_UnitIDExists(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			SubTypeCreationPredicate: subTypeCreationPredicate,
			ParentTypeID:             nil,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), fmt.Sprintf("unit %s exists", nftTypeID1))
}

func TestExecuteDefineNFT_ParentDoesNotExist(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), fmt.Sprintf("item %s does not exist", parent1Identifier))
}

func TestExecuteDefineNFT_InvalidParentType(t *testing.T) {
	txs, s, _ := newTokenTxSystem(t)
	require.NoError(t, s.Apply(state.AddUnit(parent1Identifier, &mockUnitData{})))
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   symbol,
			ParentTypeID:             parent1Identifier,
			SubTypeCreationPredicate: subTypeCreationPredicate,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{{0}}}),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.EqualError(t, txr.ServerMetadata.ErrDetail(), fmt.Sprintf("transaction validation error (type=2): token type SubTypeCreationPredicate: read [0] unit ID %q data: expected unit %[1]v data to be %T got %T", parent1Identifier, &tokens.NonFungibleTokenTypeData{}, &mockUnitData{}))
}

func TestExecuteDefineNFT_InvalidPartitionID(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(0),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
	)
	txr, err := txs.Execute(tx)
	require.EqualError(t, err, "invalid transaction: error invalid partition identifier")
	require.Nil(t, txr)
}

func TestExecuteDefineNFT_InvalidTxType(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{}),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "unknown transaction type")
}

func TestRevertTransaction_Ok(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{ParentTypeID: nil}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.NoError(t, txr.ServerMetadata.ErrDetail())
	txs.Revert()

	_, err = txs.State().GetUnit(nftTypeID1, false)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", nftTypeID1))
}

func TestExecuteDefineNFT_InvalidSymbolLength(t *testing.T) {
	s := "♥ Alphabill ♥"
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{Symbol: s}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), ErrStrInvalidSymbolLength)
}

func TestExecuteDefineNFT_InvalidNameLength(t *testing.T) {
	n := "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill♥♥"
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol: symbol,
			Name:   n,
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), ErrStrInvalidNameLength)
}

func TestExecuteDefineNFT_InvalidIconTypeLength(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol: symbol,
			Icon:   &tokens.Icon{Type: invalidIconType, Data: []byte{1, 2, 3}},
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), ErrStrInvalidIconTypeLength)
}

func TestExecuteDefineNFT_InvalidIconDataLength(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol: symbol,
			Icon:   &tokens.Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
		}),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), ErrStrInvalidIconDataLength)
}

func TestMintNFT_Ok(t *testing.T) {
	mintingSigner, mintingVerifier := testsig.CreateSignerAndVerifier(t)
	mintingPublicKey, err := mintingVerifier.MarshalPublicKey()
	require.NoError(t, err)

	txs, _, pdr := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
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

	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))

	// set minting predicate
	ownerProof := testsig.NewAuthProofSignature(t, tx, mintingSigner)
	authProof := tokens.MintNonFungibleTokenAuthProof{TokenMintingProof: ownerProof}
	require.NoError(t, tx.SetAuthProof(authProof))

	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	u, err := txs.State().GetUnit(tx.UnitID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())

	// verify unit log was added
	require.Len(t, u.Logs(), 1)

	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.TypeID)
	require.Equal(t, nftName, d.Name)
	require.EqualValues(t, []byte{10}, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(0), d.Counter)
}

func TestMintNFT_UnitIDIsNil(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithUnitID(nil),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.EqualError(t, err, `invalid transaction: expected 33 byte unit ID, got 0 bytes`)
	require.Nil(t, txr)
}

func TestMintNFT_UnitIDHasWrongType(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithUnitID(invalidNonFungibleTokenID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), ErrStrInvalidUnitID)
}

func TestMintNFT_AlreadyExists(t *testing.T) {
	txs, s, pdr := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.NoError(t, txr.ServerMetadata.ErrDetail())

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))

	err = s.Apply(state.AddUnit(tx.UnitID, tokens.NewNonFungibleTokenData(nftTypeID2, &tokens.MintNonFungibleTokenAttributes{})))
	require.NoError(t, err)

	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "token already exists")
}

func TestMintNFT_NameLengthIsInvalid(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			Name:   test.RandomString(maxNameLength + 1),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), ErrStrInvalidNameLength)
}

func TestMintNFT_URILengthIsInvalid(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithClientMetadata(defaultClientMetadata),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			URI:    test.RandomString(4097),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
	)
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "URI exceeds the maximum allowed size of 4096 KB")
}

func TestMintNFT_URIFormatIsInvalid(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			URI:    "invalid_uri",
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "URI invalid_uri is invalid")
}

func TestMintNFT_DataLengthIsInvalid(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			TypeID: nftTypeID1,
			URI:    validNFTURI,
			Data:   test.RandomBytes(dataMaxSize + 1),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "data exceeds the maximum allowed size of 65536 KB")
}

func TestMintNFT_NFTTypeDoesNotExist(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.MintNonFungibleTokenAttributes{
			URI:    validNFTURI,
			Data:   []byte{0, 0, 0, 0},
			TypeID: tokenid.NewNonFungibleTokenTypeID(t),
		}),
		testtransaction.WithAuthProof(&tokens.MintNonFungibleTokenAuthProof{
			TokenMintingProof: []byte{0}},
		),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "nft type does not exist")
}

func TestTransferNFT_UnitDoesNotExist(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)

	nonExistingUnitID := tokenid.NewNonFungibleTokenID(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nonExistingUnitID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), fmt.Sprintf("item %s does not exist", nonExistingUnitID))
}

func TestTransferNFT_UnitIsNotNFT(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "transaction validation error (type=6): invalid type ID: expected type 0X4, got 0X2")
}

func TestTransferNFT_InvalidCounter(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           1,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{OwnerProof: templates.EmptyArgument()}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "invalid counter")
}

func TestTransferNFT_InvalidTypeID(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            tokenid.NewFungibleTokenTypeID(t),
			NewOwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: []byte{0, 0, 0, 1},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "invalid type identifier")
}

func TestTransferNFT_EmptyTypeID(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
}

func createClientMetadata() *types.ClientMetadata {
	return &types.ClientMetadata{
		Timeout:           1000,
		MaxTransactionFee: 10,
		FeeCreditRecordID: feeCreditID,
	}
}

func TestTransferNFT_InvalidPredicateFormat(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// transfer NFT from 'always true' to 'p2pkh'
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "transaction validation error (type=6): evaluating owner predicate: decoding predicate:")
}

func TestTransferNFT_InvalidSignature(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// transfer with invalid signature
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),

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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.EqualError(t, txr.ServerMetadata.ErrDetail(), `transaction validation error (type=6): evaluating owner predicate: executing predicate: "always true" predicate arguments must be empty`)
}

func TestTransferNFT_Ok(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// transfer NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)

	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.TypeID)
	require.Equal(t, nftName, d.Name)
	require.EqualValues(t, []byte{10}, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(1), d.Counter)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.Owner())
}

func TestTransferNFT_BurnedBearerMustFail(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// transfer NFT, set bearer to un-spendable predicate
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)

	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	require.EqualValues(t, templates.AlwaysFalseBytes(), u.Data().Owner())

	// the token must be considered as burned and not transferable
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),

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
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "evaluating owner predicate: predicate evaluated to \"false\"")
}

func TestTransferNFT_LockedToken(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeLockToken),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)

	// verify unit was locked
	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	tokenData := u.Data().(*tokens.NonFungibleTokenData)
	require.EqualValues(t, 1, tokenData.Locked)

	// update nft
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "token is locked")
}

func TestUpdateNFT_DataLengthIsInvalid(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(dataMaxSize + 1),
			Counter: 0,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "data exceeds the maximum allowed size of 65536 KB")
}

func TestUpdateNFT_UnitDoesNotExist(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftUnitID, err := pdr.ComposeUnitID(types.ShardID{}, tokens.NonFungibleTokenUnitType, func(b []byte) error { return nil })
	require.NoError(t, err)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(nftUnitID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(0),
			Counter: 0,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), fmt.Sprintf("item %s does not exist", nftUnitID))
}

func TestUpdateNFT_UnitIsNotNFT(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(nftTypeID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 0,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "invalid unit ID")
}

func TestUpdateNFT_LockedToken(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// lock token
	lockTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeLockToken),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{lockTx.UnitID, feeCreditID}, txr.TargetUnits())

	// verify unit was locked
	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	tokenData := u.Data().(*tokens.NonFungibleTokenData)
	require.EqualValues(t, 1, tokenData.Locked)

	// update nft
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 0,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "token is locked")
}

func TestUpdateNFT_InvalidCounter(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 1,
		}),
		testtransaction.WithAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.ErrorContains(t, txr.ServerMetadata.ErrDetail(), "invalid counter")
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
}

func TestUpdateNFT_InvalidSignature(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)

	// create NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID2),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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

	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.EqualValues(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.NoError(t, txr.ServerMetadata.ErrDetail())

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
	nftID := tx.UnitID

	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.NoError(t, txr.ServerMetadata.ErrDetail())
	require.EqualValues(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())

	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{feeCreditID}, txr.TargetUnits())
	require.EqualError(t, txr.ServerMetadata.ErrDetail(), `transaction validation error (type=12): data update predicate: executing predicate: failed to decode P2PKH256 signature: cbor: cannot unmarshal positive integer into Go value of type templates.P2pkh256Signature`)
}

func TestUpdateNFT_Ok(t *testing.T) {
	txs, _, pdr := newTokenTxSystem(t)
	nftID := defineNFTAndMintToken(t, txs, &pdr, nftTypeID2)

	// update NFT
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(nftID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{nftID, feeCreditID}, txr.TargetUnits())

	u, err := txs.State().GetUnit(nftID, false)
	require.NoError(t, err)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.TypeID)
	require.Equal(t, nftName, d.Name)
	require.EqualValues(t, updatedData, d.Data)
	require.Equal(t, validNFTURI, d.URI)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.DataUpdatePredicate)
	require.Equal(t, uint64(1), d.Counter)
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.Owner())
}

// Test LockFC -> UnlockFC
func TestExecute_LockFeeCreditTxs_OK(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t)

	err := txs.BeginBlock(1)
	require.NoError(t, err)

	// lock fee credit record
	signer, _ := testsig.CreateSignerAndVerifier(t)
	lockFCAttr := testutils.NewLockFCAttr(testutils.WithLockFCCounter(10))
	lockFC := testutils.NewLockFC(t, signer, lockFCAttr,
		testtransaction.WithUnitID(feeCreditID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
	)
	txr, err := txs.Execute(lockFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{lockFC.UnitID}, txr.TargetUnits())

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
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
	)
	txr, err = txs.Execute(unlockFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{unlockFC.UnitID}, txr.TargetUnits())

	// verify unit was unlocked
	fcrUnit, err := txs.State().GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcr, ok = fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.False(t, fcr.IsLocked())
}

func TestExecute_FailedTxInFeelessMode(t *testing.T) {
	txs, _, _ := newTokenTxSystem(t,
		WithAdminOwnerPredicate(templates.AlwaysTrueBytes()),
		WithFeelessMode(true))

	// lock fee credit record (not supported in feeless mode)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	lockFCAttr := testutils.NewLockFCAttr(testutils.WithLockFCCounter(10))

	lockFC := testutils.NewLockFC(t, signer, lockFCAttr,
		testtransaction.WithUnitID(feeCreditID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)

	// Failed tx in feeless mode does not change state
	ss, err := txs.StateSummary()
	require.NoError(t, err)
	rootHashBefore := ss.Root()

	u, err := txs.State().GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcrBefore, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)

	txr, err := txs.Execute(lockFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	require.EqualValues(t, 0, txr.ServerMetadata.ActualFee)

	u, err = txs.State().GetUnit(feeCreditID, false)
	require.NoError(t, err)
	fcrAfter, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, fcrBefore.Balance, fcrAfter.Balance)

	ss, err = txs.EndBlock()
	require.NoError(t, err)
	require.Equal(t, rootHashBefore, ss.Root())
}

func defineNFTAndMintToken(t *testing.T, txs *txsystem.GenericTxSystem, pdr *types.PartitionDescriptionRecord, nftTypeID types.UnitID) types.UnitID {
	// define NFT type
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithUnitID(nftTypeID),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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

	txr, err := txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	// mint NFT
	tx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeMintNFT),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
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
	require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, pdr))
	txr, err = txs.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{tx.UnitID, feeCreditID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	return tx.UnitID
}

type mockUnitData struct{}

func (m mockUnitData) Write(hasher abhash.Hasher) { hasher.Write(&m) }

func (m mockUnitData) SummaryValueInput() uint64 {
	return 0
}

func (m mockUnitData) Copy() types.UnitData {
	return &mockUnitData{}
}

func (m mockUnitData) Owner() []byte {
	return nil
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
	sigBytes, err := tx.AuthProofSigBytes()
	require.NoError(t, err)
	signature, err := signer.SignBytes(sigBytes)
	require.NoError(t, err)
	return signature, templates.NewP2pkh256SignatureBytes(signature, pubKey)
}

func newTokenTxSystem(t *testing.T, opts ...Option) (*txsystem.GenericTxSystem, *state.State, types.PartitionDescriptionRecord) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(state.AddUnit(feeCreditID, &fc.FeeCreditRecord{
		Balance:        100,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        10,
		MinLifetime:    1000,
	})))
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(&types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
		Version:      1,
		RoundNumber:  1,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}))
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: tokens.DefaultPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2000 * time.Millisecond,
	}

	opts = append(opts, WithTrustBase(testtb.NewTrustBase(t, verifier)), WithState(s))
	txs, err := NewTxSystem(
		pdr,
		types.ShardID{},
		observability.Default(t),
		opts...,
	)
	require.NoError(t, err)
	return txs, s, pdr
}
