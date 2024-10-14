package tokens

import (
	"testing"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/txsystem"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

// TestTransferNFT_StateLock locks NFT with a transfer tx to pk1, then unlocks it with an update tx
func TestTransferNFT_StateLock(t *testing.T) {
	w1Signer, w1PubKey := createSigner(t)
	txs, _ := newTokenTxSystem(t)
	unitID := defineNFTAndMintToken(t, txs, nftTypeID2)

	// transfer NFT to pk1 with state lock
	transferTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferNFT),
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			TypeID:            nftTypeID2,
			NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(w1PubKey)),
			Counter:           0,
		}),
		testtransaction.WithAuthProof(tokens.TransferNonFungibleTokenAuthProof{
			TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}},
		),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithStateLock(&types.StateLock{
			ExecutionPredicate: templates.NewP2pkh256BytesFromKey(w1PubKey),
			RollbackPredicate:  templates.NewP2pkh256BytesFromKey(w1PubKey)},
		),
	)
	_, err := txs.Execute(transferTx)
	require.NoError(t, err)

	// verify unit was locked and bearer hasn't changed
	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	require.True(t, u.IsStateLocked())

	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, nftTypeID2, d.TypeID)
	require.Equal(t, []byte{0xa}, d.Data)
	require.Equal(t, uint64(0), d.Counter)
	require.Equal(t, templates.AlwaysTrueBytes(), u.Owner())

	// try to update nft without state unlocking
	updateTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 1,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	sm, err := txs.Execute(updateTx)
	require.NoError(t, err)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)

	// update nft with state unlock, it must be transferred to new bearer w1
	attr := &tokens.UpdateNonFungibleTokenAttributes{
		Data:    []byte{42},
		Counter: 1,
	}
	updateTx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(tokens.TransactionTypeUpdateNFT),
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
		testtransaction.WithAuthProof(
			tokens.UpdateNonFungibleTokenAuthProof{TokenTypeDataUpdateProofs: [][]byte{templates.EmptyArgument()}},
		),
	)
	ownerProof := testsig.NewStateLockProofSignature(t, updateTx, w1Signer)
	updateTx.StateUnlock = append([]byte{byte(txsystem.StateUnlockExecute)}, ownerProof...)

	_, err = txs.Execute(updateTx)
	require.NoError(t, err)
	require.Equal(t, types.TxStatusFailed, sm.SuccessIndicator)

	// verify unit was unlocked and bearer has changed
	u, err = txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	require.False(t, u.IsStateLocked())

	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d = u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, nftTypeID2, d.TypeID)
	require.Equal(t, attr.Data, d.Data)
	require.Equal(t, uint64(2), d.Counter)
	require.Equal(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(w1PubKey)), u.Owner())
}
