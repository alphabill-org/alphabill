package tokens

import (
	gocrypto "crypto"
	"testing"

	hasherUtil "github.com/alphabill-org/alphabill/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/predicates/templates"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestTransferNFT_StateLock(t *testing.T) {
	w1Signer, w1PubKey := createSigner(t)
	//w2Signer, w2PubKey := createSigner(t)
	_ = w1Signer
	txs, _ := newTokenTxSystem(t)
	mintTx := createNFTTypeAndMintToken(t, txs, nftTypeID2, unitID)

	// transfer NFT to pk1 with state lock
	transferTx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(PayloadTypeTransferNFT),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NFTTypeID:                    nftTypeID2,
			NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(hasherUtil.Sum256(w1PubKey)),
			Nonce:                        test.RandomBytes(32),
			Backlink:                     mintTx.Hash(gocrypto.SHA256),
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

	require.IsType(t, &NonFungibleTokenData{}, u.Data())
	d := u.Data().(*NonFungibleTokenData)
	_ = d
	require.Equal(t, zeroSummaryValue, d.SummaryValueInput())
	require.Equal(t, nftTypeID2, d.NftType)
	require.Equal(t, mintTx.Hash(gocrypto.SHA256), d.Backlink)
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())

	//require.Equal(t, templates.NewP2pkh256BytesFromKeyHash(hasherUtil.Sum256(w1PubKey)), u.Bearer())
	//
	// try to update nft without state unlocking
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
	require.ErrorContains(t, err, "unit has a state lock, but tx does not have unlock proof")

	// TODO: update nft with state unlock, it must be transferred to new bearer w1
}
