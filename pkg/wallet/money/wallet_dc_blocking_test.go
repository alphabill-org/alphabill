package money

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/txsystem"

	"github.com/alphabill-org/alphabill/pkg/wallet/account"

	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/stretchr/testify/require"
)

func TestBlockingDcWithNormalBills(t *testing.T) {
	// wallet contains 2 normal bills
	bills := []*Bill{addBill(1), addBill(2)}
	nonce := calculateDcNonce(bills)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	dcBills := []*Bill{addDcBill(t, k, bills[0].Id, nonce, 1, dcTimeoutBlockCount), addDcBill(t, k, bills[1].Id, nonce, 2, dcTimeoutBlockCount)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonce, 0, dcTimeoutBlockCount, k)...)

	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList}, am)
	tx, err := createSwapTx(k, w.SystemID(), dcBills, calculateDcNonce(bills), getBillIds(bills), swapTimeoutBlockCount)
	require.NoError(t, err)
	mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{
		tx,
	}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 0}}})

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and dc + swap txs should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 3)
	for _, tx := range mockClient.GetRecordedTransactions()[0:2] {
		dcTx := parseDcTx(t, tx)
		require.EqualValues(t, nonce, dcTx.Nonce)
	}
	swapTx := parseSwapTx(t, mockClient.GetRecordedTransactions()[2])
	require.EqualValues(t, 3, swapTx.TargetValue)

	// and expected swaps are cleared
	require.Empty(t, w.dcWg.swaps)
}

func TestBlockingDCWithDCBillsBeforeDCTimeout(t *testing.T) {
	// create wallet with 2 dc bills
	roundNr := uint64(5)
	tempNonce := uint256.NewInt(1)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, tempNonce, util.Uint256ToBytes(tempNonce), 1, dcTimeoutBlockCount), addDcBill(t, k, tempNonce, util.Uint256ToBytes(tempNonce), 2, dcTimeoutBlockCount)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, util.Uint256ToBytes(tempNonce), 0, dcTimeoutBlockCount, k)
	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList}, am)
	// set specific round number
	mockClient.SetMaxRoundNumber(roundNr)

	tx, err := createSwapTx(k, w.SystemID(), bills, util.Uint256ToBytes(tempNonce), getBillIds(bills), swapTimeoutBlockCount)
	require.NoError(t, err)
	mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{
		tx,
	}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: roundNr}}})

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and swap tx should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	swapTx := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, swapTx.TargetValue)

	// and expected swaps are cleared
	require.Empty(t, w.dcWg.swaps)
}

func TestBlockingDCWithExistingExpiredDCBills(t *testing.T) {
	// create wallet with 2 timed out dc bills
	tempNonce := uint256.NewInt(1)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, tempNonce, util.Uint256ToBytes(tempNonce), 1, 0), addDcBill(t, k, tempNonce, util.Uint256ToBytes(tempNonce), 2, 0)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, util.Uint256ToBytes(tempNonce), 0, 0, k)
	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList}, am)

	tx, err := createSwapTx(k, w.SystemID(), bills, util.Uint256ToBytes(tempNonce), getBillIds(bills), swapTimeoutBlockCount)
	require.NoError(t, err)
	mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{
		tx,
	}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 0}}})

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and swap tx should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	swapTx := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, swapTx.TargetValue)

	// and expected swaps are cleared
	require.Empty(t, w.dcWg.swaps)
}

func TestBlockingDcWaitingForSwapTimesOut(t *testing.T) {
	// wallet contains 2 normal bills
	bills := []*Bill{addBill(1), addBill(2)}
	nonce := calculateDcNonce(bills)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	dcBills := []*Bill{addDcBill(t, k, bills[0].Id, nonce, 1, dcTimeoutBlockCount), addDcBill(t, k, bills[1].Id, nonce, 2, dcTimeoutBlockCount)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonce, 0, dcTimeoutBlockCount, k)...)

	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList}, am)
	// fill empty blocks until timeout
	for i := 0; i < swapTimeoutBlockCount+1; i++ {
		mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: uint64(i)}}})
	}

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and dc + swap txs should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 3)
	for _, tx := range mockClient.GetRecordedTransactions()[0:2] {
		dcTx := parseDcTx(t, tx)
		require.EqualValues(t, nonce, dcTx.Nonce)
	}
	swapTx := parseSwapTx(t, mockClient.GetRecordedTransactions()[2])
	require.EqualValues(t, 3, swapTx.TargetValue)

	// and expected swaps are cleared
	require.Empty(t, w.dcWg.swaps)
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

func waitForExpectedSwap(w *Wallet) {
	waitForCondition(func() bool {
		w.dcWg.mu.Lock()
		defer w.dcWg.mu.Unlock()
		return len(w.dcWg.swaps) > 0
	})
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
