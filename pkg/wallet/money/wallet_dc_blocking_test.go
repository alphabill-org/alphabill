package money

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestBlockingDcWithNormalBills(t *testing.T) {
	// wallet contains 2 normal bills
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)

	k, _ := w.db.Do().GetAccountKey(0)

	// when blocking dust collector runs
	wg := runBlockingDc(t, w)

	// then expected swap data should be saved
	waitForExpectedSwap(w)

	// and dc txs should be sent
	dcNonce := calculateExpectedDcNonce(t, w)
	require.Len(t, mockClient.GetRecordedTransactions(), 2)
	for _, tx := range mockClient.GetRecordedTransactions() {
		dcTx := parseDcTx(t, tx)
		require.EqualValues(t, dcNonce, dcTx.Nonce)
	}

	// when the swap tx with given nonce is received
	res := createBlockWithSwapTx(dcNonce, k, mockClient.GetRecordedTransactions())
	err := w.ProcessBlock(res.Block)
	require.NoError(t, err)

	// then only the swapped bill should exist
	bills, _ := w.db.Do().GetBills(0)
	require.Len(t, bills, 1)
	b := bills[0]
	require.EqualValues(t, b.Value, 3)
	require.EqualValues(t, b.GetID(), dcNonce)

	// and the blocking dc should return
	wg.Wait()

	// and expected swap should be cleared
	require.Len(t, w.dcWg.swaps, 0)
}

func TestBlockingDcWithDcBills(t *testing.T) {
	// wallet contains 2 dc bills
	w, _ := CreateTestWallet(t)
	dcNonce := uint256.NewInt(1337)
	addDcBills(t, w, dcNonce, 10)
	k, _ := w.db.Do().GetAccountKey(0)

	// when blocking dust collector runs
	wg := runBlockingDc(t, w)

	// then expected swap data should be saved
	waitForExpectedSwap(w)
	require.Len(t, w.dcWg.swaps, 1)

	// when the swap tx with dc bills is received
	bills, _ := w.db.Do().GetBills(0)
	res := createBlockWithSwapTxFromDcBills(dcNonce, k, bills...)
	err := w.ProcessBlock(res.Block)
	require.NoError(t, err)

	// then only the swapped bill should exist
	bills, _ = w.db.Do().GetBills(0)
	require.Len(t, bills, 1)
	b := bills[0]
	require.EqualValues(t, b.Value, 3)
	require.EqualValues(t, b.Id, dcNonce)

	// and the blocking dc should return
	wg.Wait()

	// and expected swap should be cleared
	require.Len(t, w.dcWg.swaps, 0)
}

func TestBlockingDcWithDifferentDcBills(t *testing.T) {
	// wallet contains multiple groups of dc bills
	w, _ := CreateTestWallet(t)

	// group 1
	dcNonce1 := uint256.NewInt(101)
	b11 := addDcBill(t, w, dcNonce1, 1, 10)
	b12 := addDcBill(t, w, dcNonce1, 2, 10)

	// group 2
	dcNonce2 := uint256.NewInt(102)
	b21 := addDcBill(t, w, dcNonce2, 3, 10)
	b22 := addDcBill(t, w, dcNonce2, 4, 10)
	b23 := addDcBill(t, w, dcNonce2, 5, 10)

	k, _ := w.db.Do().GetAccountKey(0)

	// when blocking dust collector runs
	wg := runBlockingDc(t, w)

	// then expected swap data should be saved
	waitForExpectedSwap(w)
	require.Len(t, w.dcWg.swaps, 2)

	// when group 1 swap is received
	res1 := createBlockWithSwapTxFromDcBills(dcNonce1, k, b11, b12)
	res1.Block.BlockNumber = 1
	err := w.ProcessBlock(res1.Block)
	require.NoError(t, err)

	// then swap waitgroup is decremented
	require.Len(t, w.dcWg.swaps, 1)

	// and bills are updated
	bills, _ := w.db.Do().GetBills(0)
	require.Len(t, bills, 4)

	// when the swap tx with dc bills is received
	res2 := createBlockWithSwapTxFromDcBills(dcNonce2, k, b21, b22, b23)
	res2.Block.BlockNumber = 2
	err = w.ProcessBlock(res2.Block)
	require.NoError(t, err)

	// then the blocking dc should return
	wg.Wait()

	// and expected swap should be cleared
	require.Len(t, w.dcWg.swaps, 0)

	// and wallet bills are updated
	bills, _ = w.db.Do().GetBills(0)
	require.Len(t, bills, 2)
}

func TestSendingSwapUpdatesDcWaitGroupTimeout(t *testing.T) {
	// create wallet with dc bill that is timed out
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addDcBill(t, w, nonce, 2, dcTimeoutBlockCount)
	setDcMetadata(t, w, nonce32[:], &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount, SwapTimeout: 0})
	_ = w.db.Do().SetBlockNumber(dcTimeoutBlockCount)
	mockClient.SetMaxBlockNumber(dcTimeoutBlockCount)
	w.dcWg.addExpectedSwap(expectedSwap{dcNonce: nonce32[:], timeout: dcTimeoutBlockCount})

	// when trySwap is called
	err := w.trySwap(w.db.Do(), 0)
	require.NoError(t, err)

	// then dcWg timeout is updated
	require.Len(t, w.dcWg.swaps, 1)
	require.EqualValues(t, w.dcWg.swaps[*nonce], dcTimeoutBlockCount+swapTimeoutBlockCount)
}

func runBlockingDc(t *testing.T, w *Wallet) *sync.WaitGroup {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		dcErr := w.collectDust(context.Background(), true, 0)
		require.NoError(t, dcErr)
		wg.Done()
	}()
	return &wg
}

func createBlockWithSwapTxFromDcBills(dcNonce *uint256.Int, k *wallet.AccountKey, bills ...*Bill) *alphabill.GetBlockResponse {
	var dcTxs []*txsystem.Transaction
	for _, b := range bills {
		dcTxs = append(dcTxs, &txsystem.Transaction{
			SystemId:              alphabillMoneySystemId,
			UnitId:                b.GetID(),
			TransactionAttributes: moneytesttx.CreateRandomDustTransferTx(),
			Timeout:               1000,
			OwnerProof:            script.PredicateArgumentEmpty(),
		})
	}
	dcNonce32 := dcNonce.Bytes32()
	return createBlockWithSwapTx(dcNonce32[:], k, dcTxs)
}

func createBlockWithSwapTx(dcNonce []byte, k *wallet.AccountKey, dcTxs []*txsystem.Transaction) *alphabill.GetBlockResponse {
	return &alphabill.GetBlockResponse{
		Block: &block.Block{
			SystemIdentifier:  alphabillMoneySystemId,
			BlockNumber:       1,
			PreviousBlockHash: hash.Sum256([]byte{}),
			Transactions: []*txsystem.Transaction{
				{
					SystemId:              alphabillMoneySystemId,
					UnitId:                dcNonce,
					TransactionAttributes: createSwapTxFromDcTxs(k.PubKeyHash.Sha256, dcTxs),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: &certificates.UnicityCertificate{},
		},
	}
}

func waitForExpectedSwap(w *Wallet) {
	waitForCondition(func() bool {
		w.dcWg.mu.Lock()
		defer w.dcWg.mu.Unlock()
		return len(w.dcWg.swaps) > 0
	})
}

func createSwapTxFromDcTxs(pubKeyHash []byte, dcTxs []*txsystem.Transaction) *anypb.Any {
	tx, _ := anypb.New(&billtx.SwapOrder{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		BillIdentifiers: [][]byte{},
		DcTransfers:     dcTxs,
		Proofs:          []*block.BlockProof{},
		TargetValue:     3,
	})
	return tx
}

func waitForCondition(waitCondition func() bool) {
	t1 := time.Now()
	for {
		ok := waitCondition()
		if ok {
			break
		}
		if time.Since(t1).Seconds() > 5 {
			log.Warning("breaking on 5s timeout")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
