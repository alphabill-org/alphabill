package money

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/holiman/uint256"
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

	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}), am)

	var protoDcBills []*wallet.Bill
	for _, b := range dcBills {
		protoDcBills = append(protoDcBills, b.ToProto())
	}

	tx, err := txbuilder.NewSwapTx(k, w.SystemID(), protoDcBills, calculateDcNonce(bills), getBillIds(bills), swapTimeoutBlockCount)
	require.NoError(t, err)
	txRecord := &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	mockClient.SetBlock(&types.Block{
		Transactions:       []*types.TransactionRecord{txRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 0}},
	})

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
	txAttr := parseSwapTx(t, mockClient.GetRecordedTransactions()[2])
	require.EqualValues(t, 3, txAttr.TargetValue)

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
	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}), am)
	// set specific round number
	mockClient.SetMaxRoundNumber(roundNr)

	var protoBills []*wallet.Bill
	for _, b := range bills {
		protoBills = append(protoBills, b.ToProto())
	}

	txOrder, err := txbuilder.NewSwapTx(k, w.SystemID(), protoBills, util.Uint256ToBytes(tempNonce), getBillIds(bills), swapTimeoutBlockCount)
	require.NoError(t, err)
	txRecord := &types.TransactionRecord{
		TransactionOrder: txOrder,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	mockClient.SetBlock(&types.Block{
		Transactions:       []*types.TransactionRecord{txRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: roundNr}},
	})

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and swap tx should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	txAttr := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, txAttr.TargetValue)

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
	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}), am)

	var protoBills []*wallet.Bill
	for _, b := range bills {
		protoBills = append(protoBills, b.ToProto())
	}

	txOrder, err := txbuilder.NewSwapTx(k, w.SystemID(), protoBills, util.Uint256ToBytes(tempNonce), getBillIds(bills), swapTimeoutBlockCount)
	require.NoError(t, err)
	txRecord := &types.TransactionRecord{
		TransactionOrder: txOrder,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	mockClient.SetBlock(&types.Block{Transactions: []*types.TransactionRecord{
		txRecord,
	}, UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 0}}})

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and swap tx should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	txAttr := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, txAttr.TargetValue)

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

	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}), am)

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	mockClient.SetMaxRoundNumber(swapTimeoutBlockCount + 1)

	// wait for confirmation
	waitForCondition(func() bool {
		w.dcWg.mu.Lock()
		defer w.dcWg.mu.Unlock()
		return len(w.dcWg.swaps) == 0
	})

	// and dc + swap txs should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 3)
	for _, tx := range mockClient.GetRecordedTransactions()[0:2] {
		dcTx := parseDcTx(t, tx)
		require.EqualValues(t, nonce, dcTx.Nonce)
	}
	txAttr := parseSwapTx(t, mockClient.GetRecordedTransactions()[2])
	require.EqualValues(t, 3, txAttr.TargetValue)

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

//func createBlockWithSwapTxFromDcBills(dcNonce *uint256.Int, k *account.AccountKey, systemId []byte, bills ...*Bill) *alphabill.GetBlockResponse {
//	var dcTxs []*types.TxRecord
//	for _, b := range bills {
//		dcTxs = append(dcTxs, &types.TxRecord{
//			SystemId:              systemId,
//			UnitId:                b.GetID(),
//			TransactionAttributes: moneytesttx.CreateRandomDustTransferTx(),
//			OwnerProof:            script.PredicateArgumentEmpty(),
//			ClientMetadata:        &txsystem.ClientMetadata{FeeCreditRecordId: []byte{1, 3, 3, 7}, Timeout: 1000},
//			ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
//		})
//	}
//	dcNonce32 := dcNonce.Bytes32()
//	return createBlockWithSwapTx(systemId, dcNonce32[:], k, dcTxs)
//}
//
//func createBlockWithSwapTx(systemId, dcNonce []byte, k *account.AccountKey, dcTxs []*types.TxRecord) *alphabill.GetBlockResponse {
//	b := &types.Block{
//		Header: &types.Header{
//			SystemID:          systemId,
//			PreviousBlockHash: hash.Sum256([]byte{}),
//		},
//		Transactions: []*types.TxRecord{
//			{
//				TransactionOrder: &types.TransactionOrder{
//					Payload: &types.Payload{
//						SystemID:       systemId,
//						Type:           billtx.PayloadTypeSwapDC,
//						UnitID:         dcNonce,
//						Attributes:     createSwapTxFromDcTxs(k.PubKeyHash.Sha256, dcTxs),
//						ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: []byte{1, 3, 3, 7}, Timeout: 1000},
//					},
//					OwnerProof: script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
//				},
//				ServerMetadata: &types.ServerMetadata{ActualFee: 1},
//			},
//		},
//		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
//	}
//	blockBytes, _ := cbor.Marshal(b)
//	return &alphabill.GetBlockResponse{
//		Block: blockBytes,
//	}
//}

func waitForExpectedSwap(w *Wallet) {
	waitForCondition(func() bool {
		w.dcWg.mu.Lock()
		defer w.dcWg.mu.Unlock()
		return len(w.dcWg.swaps) > 0
	})
}

//func createSwapTxFromDcTxs(pubKeyHash []byte, dcTxs []*types.TxRecord) []byte {
//	attr := &billtx.SwapDCAttributes{
//		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
//		BillIdentifiers: [][]byte{},
//		DcTransfers:     dcTxs,
//		Proofs:          []*types.TxProof{},
//		TargetValue:     3,
//	}
//	attrBytes, _ := cbor.Marshal(attr)
//	return attrBytes
//}

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
