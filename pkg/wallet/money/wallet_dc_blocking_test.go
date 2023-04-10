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
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestBlockingDcWithNormalBills(t *testing.T) {
	// wallet contains 2 normal bills
	bills := []*Bill{addBill(1), addBill(2)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount)

	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList})

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// then expected swap data should be saved
	waitForExpectedSwap(w)

	// and dc txs should be sent
	dcNonce := calculateDcNonce(bills)
	require.Len(t, mockClient.GetRecordedTransactions(), 2)
	for _, tx := range mockClient.GetRecordedTransactions() {
		dcTx := parseDcTx(t, tx)
		require.EqualValues(t, dcNonce, dcTx.Nonce)
	}
}

func TestBlockingDcWithDcBills(t *testing.T) {
	// wallet contains 2 dc bills
	w, _ := CreateTestWallet(t, nil)
	k, _ := w.am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, w, k, uint256.NewInt(1), 1, dcTimeoutBlockCount), addDcBill(t, w, k, uint256.NewInt(1), 2, dcTimeoutBlockCount)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount)

	w, _ = CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList})

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// then expected swap data should be saved
	waitForExpectedSwap(w)
	require.Len(t, w.dcWg.swaps, 1)
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

func createBlockWithSwapTxFromDcBills(dcNonce *uint256.Int, k *account.AccountKey, systemId []byte, bills ...*Bill) *alphabill.GetBlockResponse {
	var dcTxs []*txsystem.Transaction
	for _, b := range bills {
		dcTxs = append(dcTxs, &txsystem.Transaction{
			SystemId:              systemId,
			UnitId:                b.GetID(),
			TransactionAttributes: moneytesttx.CreateRandomDustTransferTx(),
			OwnerProof:            script.PredicateArgumentEmpty(),
			ClientMetadata:        &txsystem.ClientMetadata{FeeCreditRecordId: []byte{1, 3, 3, 7}, Timeout: 1000},
			ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
		})
	}
	dcNonce32 := dcNonce.Bytes32()
	return createBlockWithSwapTx(systemId, dcNonce32[:], k, dcTxs)
}

func createBlockWithSwapTx(systemId, dcNonce []byte, k *account.AccountKey, dcTxs []*txsystem.Transaction) *alphabill.GetBlockResponse {
	return &alphabill.GetBlockResponse{
		Block: &block.Block{
			SystemIdentifier:  systemId,
			PreviousBlockHash: hash.Sum256([]byte{}),
			Transactions: []*txsystem.Transaction{
				{
					SystemId:              systemId,
					UnitId:                dcNonce,
					TransactionAttributes: createSwapTxFromDcTxs(k.PubKeyHash.Sha256, dcTxs),
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
					ClientMetadata:        &txsystem.ClientMetadata{FeeCreditRecordId: []byte{1, 3, 3, 7}, Timeout: 1000},
					ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
				},
			},
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
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
	tx, _ := anypb.New(&billtx.SwapDCAttributes{
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
