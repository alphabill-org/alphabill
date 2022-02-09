package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/alphabill/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/crypto/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/rpc/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
	"testing"
	"time"
)

func TestBlockingDcWithNormalBills(t *testing.T) {
	// wallet contains 2 normal bills
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)

	k, _ := w.db.GetAccountKey()

	// when blocking dust collector runs
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		dcErr := w.collectDust(true)
		require.NoError(t, dcErr)
		wg.Done()
	}()

	// then expected swap data should be saved
	waitForExpectedSwapToBeSaved(w)

	// and dc txs should be sent
	dcNonce := calculateExpectedDcNonce(t, w)
	require.Len(t, mockClient.txs, 2)
	for _, tx := range mockClient.txs {
		dcTx := parseDcTx(t, tx)
		require.EqualValues(t, dcNonce, dcTx.Nonce)
	}

	// when the swap tx with given nonce is received
	block := createBlockWithSwapTx(dcNonce, k, mockClient.txs)
	err := w.processBlock(block)
	require.NoError(t, err)

	// then only the swapped bill should exist
	bills, _ := w.db.GetBills()
	require.Len(t, bills, 1)
	b := bills[0]
	require.EqualValues(t, b.Value, 3)
	require.EqualValues(t, b.Id, uint256.NewInt(0).SetBytes(dcNonce))

	// when the block's post processor runs
	err = w.postProcessBlock(block)
	require.NoError(t, err)

	// then the blocking dc should return
	wg.Wait()

	// and expected swap should be cleared
	require.Len(t, w.dcWg.swaps, 0)

}

func createBlockWithSwapTx(dcNonce []byte, k *accountKey, dcTxs []*transaction.Transaction) *alphabill.GetBlocksResponse {
	return &alphabill.GetBlocksResponse{
		Block: &alphabill.Block{
			BlockNo:       1,
			PrevBlockHash: hash.Sum256([]byte{}),
			Transactions: []*transaction.Transaction{
				{
					UnitId:                dcNonce,
					TransactionAttributes: createSwapTxFromDcTxs(k.PubKeyHashSha256, dcTxs),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: []byte{},
		},
	}
}

func waitForExpectedSwapToBeSaved(w *Wallet) {
	waitForCondition(func() bool {
		ok := false
		_ = w.db.WithTransaction(func() error {
			// do in transaction because dcWg is synchronized on walletdb write lock
			if len(w.dcWg.swaps) > 0 {
				ok = true
			}
			return nil
		})
		return ok
	})
}

func createSwapTxFromDcTxs(pubKeyHash []byte, dcTxs []*transaction.Transaction) *anypb.Any {
	tx, _ := anypb.New(&transaction.Swap{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		BillIdentifiers: [][]byte{},
		DcTransfers:     dcTxs,
		Proofs:          [][]byte{},
		TargetValue:     3,
	})
	return tx
}

func waitForCondition(waitCondition func() bool) {
	t1 := time.Now()
	for {
		if waitCondition() {
			return
		}
		if time.Since(t1).Seconds() > 5 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}
