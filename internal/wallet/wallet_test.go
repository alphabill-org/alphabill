package wallet

import (
	"alphabill-wallet-sdk/internal/alphabill/script"
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWalletCanSendTx(t *testing.T) {
	w, err := CreateNewWallet()
	defer cleanup(w)
	require.NoError(t, err)

	w.syncWithAlphaBill(&mockAlphaBillClient{})
	b := bill{
		Id:     *uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.AddBill(&b)
	require.NoError(t, err)

	receiverPubKey := make([]byte, 33)
	err = w.Send(receiverPubKey, 50)
	require.NoError(t, err)
}

func TestBlockProcessing(t *testing.T) {
	w, err := CreateNewWallet()
	defer cleanup(w)
	require.NoError(t, err)

	k, err := w.db.GetKey()
	require.NoError(t, err)

	blocks := []*alphabill.Block{
		{
			BlockNo:       1,
			PrevBlockHash: hash.Sum256([]byte{}),
			Transactions: []*transaction.Transaction{
				// random dust transfer can be processed
				{
					UnitId:                hash.Sum256([]byte{0x00}),
					TransactionAttributes: createDustTransferTx(),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentEmpty(),
				},
				// receive transfer of 100 bills
				{
					UnitId:                hash.Sum256([]byte{0x01}),
					TransactionAttributes: createBillTransferTx(k.PubKeyHashSha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive split of 100 bills
				{
					UnitId:                hash.Sum256([]byte{0x02}),
					TransactionAttributes: createBillSplitTx(k.PubKeyHashSha256, 100, 100),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive swap of 100 bills
				{
					UnitId:                hash.Sum256([]byte{0x03}),
					TransactionAttributes: createSwapTx(k.PubKeyHashSha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: []byte{},
		},
	}

	require.EqualValues(t, 0, w.db.GetBlockHeight())
	require.EqualValues(t, 0, w.db.GetBalance())
	for _, block := range blocks {
		err := w.processBlock(block)
		require.NoError(t, err)
	}
	require.EqualValues(t, 300, w.GetBalance())
	require.EqualValues(t, 1, w.db.GetBlockHeight())
}

func cleanup(w *Wallet) {
	if w != nil {
		w.Shutdown()
		_ = w.DeleteDb()
	}
}
