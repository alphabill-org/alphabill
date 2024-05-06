package tokens

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/txsystem"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

// TestTransferNFT_StateLock locks NFT with a transfer tx to pk1, then unlocks it with an update tx
func TestTransferNFT_StateLock(t *testing.T) {
	w1Signer, w1PubKey := createSigner(t)
	_ = w1Signer
	txs, _ := newTokenTxSystem(t)
	unitID := createNFTTypeAndMintToken(t, txs, nftTypeID2)

	// transfer NFT to pk1 with state lock
	transferTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeTransferNFT),
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(w1PubKey)),
			Nonce:                        test.RandomBytes(32),
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{nil},
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	transferTx.Payload.StateLock = &types.StateLock{
		ExecutionPredicate: templates.NewP2pkh256BytesFromKey(w1PubKey),
	}
	_, err := txs.Execute(transferTx)
	require.NoError(t, err)

	// verify unit was locked and bearer hasn't changed
	u, err := txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	require.True(t, u.IsStateLocked())

	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, []byte{0xa}, d.Data)
	require.Equal(t, uint64(0), d.Counter)
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())

	// try to update nft without state unlocking
	updateTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(&tokens.UpdateNonFungibleTokenAttributes{
			Data:    test.RandomBytes(10),
			Counter: 1,
		}),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(nil),
	)
	_, err = txs.Execute(updateTx)
	require.ErrorContains(t, err, "unit has a state lock, but tx does not have unlock proof")

	// update nft with state unlock, it must be transferred to new bearer w1
	attr := &tokens.UpdateNonFungibleTokenAttributes{
		Data:                 []byte{42},
		Counter:              1,
		DataUpdateSignatures: [][]byte{nil, nil},
	}
	updateTx = testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(tokens.PayloadTypeUpdateNFT),
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(createClientMetadata()),
		testtransaction.WithFeeProof(templates.EmptyArgument()),
	)
	require.NoError(t, updateTx.SetOwnerProof(predicates.OwnerProoferForSigner(w1Signer)))
	updateTx.StateUnlock = append([]byte{byte(txsystem.StateUnlockExecute)}, updateTx.OwnerProof...)

	_, err = txs.Execute(updateTx)
	require.NoError(t, err, "failed to execute update tx")

	// verify unit was unlocked and bearer has changed
	u, err = txs.State().GetUnit(unitID, false)
	require.NoError(t, err)
	require.False(t, u.IsStateLocked())

	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d = u.Data().(*tokens.NonFungibleTokenData)
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, attr.Data, d.Data)
	require.Equal(t, uint64(2), d.Counter)
	require.Equal(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(w1PubKey)), u.Bearer())
}
