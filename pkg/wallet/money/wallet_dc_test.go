package money

import (
	"context"
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestDustCollectionWontRunForSingleBill(t *testing.T) {
	// create wallet with a single bill
	bills := []*Bill{addBill(1)}
	billsList := createBillListJsonResponse(bills)

	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{customBillList: billsList})

	// when dc runs
	counter := uint64(0)
	_, err := w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	// then no txs are broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 0)
}

func TestDustCollectionMaxBillCount(t *testing.T) {
	// create wallet with max allowed bills for dc + 1
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := make([]*Bill, maxBillsForDustCollection+1)
	dcBills := make([]*Bill, maxBillsForDustCollection+1)
	for i := 0; i < maxBillsForDustCollection+1; i++ {
		bills[i] = addBill(uint64(i))
		dcBills[i] = addDcBill(t, k, uint256.NewInt(uint64(i)), nonceBytes, uint64(i), dcTimeoutBlockCount)
	}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonceBytes, 0, dcTimeoutBlockCount, k)...)

	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		},
	})

	// when dc runs
	counter := uint64(0)
	_, err = w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	// then dc tx count should be equal to max allowed bills for dc plus 1 for the swap
	require.Len(t, mockClient.GetRecordedTransactions(), maxBillsForDustCollection+1)
}

func TestDustCollectionMaxBillCountInitialized(t *testing.T) {
	// define that some dc bills have already been processed
	counter := uint64(95)
	// create wallet with one bill over the limit
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	billsNumber := int(maxBillsForDustCollection + 1 - counter)
	bills := make([]*Bill, billsNumber)
	dcBills := make([]*Bill, billsNumber)
	for i := 0; i < billsNumber; i++ {
		bills[i] = addBill(uint64(i))
		dcBills[i] = addDcBill(t, k, uint256.NewInt(uint64(i)), nonceBytes, uint64(i), dcTimeoutBlockCount)
	}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonceBytes, 0, dcTimeoutBlockCount, k)...)

	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		},
	})

	// when dc runs
	_, err = w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	// then dc tx count should be equal to max allowed bills minus the counter plus 1 for the swap
	require.Len(t, mockClient.GetRecordedTransactions(), billsNumber)
}

func TestBasicDustCollection(t *testing.T) {
	// create wallet with 2 normal bills
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	dcBills := []*Bill{addDcBill(t, k, uint256.NewInt(1), nonceBytes, 1, dcTimeoutBlockCount), addDcBill(t, k, uint256.NewInt(2), nonceBytes, 2, dcTimeoutBlockCount)}
	bills := []*Bill{addBill(1), addBill(2)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonceBytes, 0, dcTimeoutBlockCount, k)...)
	expectedDcNonce := calculateDcNonce(bills)

	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		}}, am)

	// when dc runs
	counter := uint64(0)
	_, err = w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	// then two dc txs are broadcast plus one swap
	require.Len(t, mockClient.GetRecordedTransactions(), 3)
	for i, tx := range mockClient.GetRecordedTransactions()[0:2] {
		dcTx := parseDcTx(t, tx)
		require.NotNil(t, dcTx)
		require.EqualValues(t, expectedDcNonce, dcTx.Nonce)
		require.EqualValues(t, bills[i].Value, dcTx.TargetValue)
		require.EqualValues(t, bills[i].TxHash, dcTx.Backlink)
		require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), dcTx.TargetBearer)
	}

	// and expected swap is added to dc wait group
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(expectedDcNonce)]
	require.EqualValues(t, expectedDcNonce, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, dcTimeoutBlockCount, swap.timeout)
}

func TestDustCollectionWithSwap(t *testing.T) {
	// create wallet with 2 normal bills
	tempNonce := uint256.NewInt(1)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addBill(1), addBill(2)}
	expectedDcNonce := calculateDcNonce(bills)
	billsList := createBillListJsonResponse(bills)
	// proofs are polled twice, one for the regular bills and one for dc bills
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, k)
	proofList = append(proofList, createBlockProofJsonResponse(t, []*Bill{addDcBill(t, k, tempNonce, expectedDcNonce, 1, dcTimeoutBlockCount), addDcBill(t, k, tempNonce, expectedDcNonce, 2, dcTimeoutBlockCount)}, expectedDcNonce, 0, dcTimeoutBlockCount, k)...)

	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		},
	}, am)

	// when dc runs
	counter := uint64(0)
	_, err = w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	// then two dc txs + one swap tx are broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 3)
	for _, tx := range mockClient.GetRecordedTransactions()[0:2] {
		require.NotNil(t, parseDcTx(t, tx))
	}
	txSwap := parseSwapTx(t, mockClient.GetRecordedTransactions()[2])
	require.EqualValues(t, 3, txSwap.TargetValue)
	require.EqualValues(t, [][]byte{util.Uint256ToBytes(tempNonce), util.Uint256ToBytes(tempNonce)}, txSwap.BillIdentifiers)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
	require.Len(t, txSwap.DcTransfers, 2)
	require.Len(t, txSwap.Proofs, 2)

	// and expected swap is updated with swap timeout
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(expectedDcNonce)]
	require.EqualValues(t, expectedDcNonce, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, swapTimeoutBlockCount, swap.timeout)
}

func TestSwapWithExistingDCBillsBeforeDCTimeout(t *testing.T) {
	// create wallet with 2 dc bills
	roundNr := uint64(5)
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, tempNonce, nonceBytes, 1, dcTimeoutBlockCount), addDcBill(t, k, tempNonce, nonceBytes, 2, dcTimeoutBlockCount)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nonceBytes, 0, dcTimeoutBlockCount, k)
	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		}}, am)
	// set specific round number
	mockClient.SetMaxRoundNumber(roundNr)

	// when dc runs
	counter := uint64(0)
	_, err = w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	// then a swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	txSwap := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, txSwap.TargetValue)
	require.EqualValues(t, [][]byte{nonceBytes, nonceBytes}, txSwap.BillIdentifiers)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
	require.Len(t, txSwap.DcTransfers, 2)
	require.Len(t, txSwap.Proofs, 2)

	// and expected swap is updated with swap timeout + round number
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(nonceBytes)]
	require.EqualValues(t, nonceBytes, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, swapTimeoutBlockCount+roundNr, swap.timeout)
}

func TestSwapWithExistingExpiredDCBills(t *testing.T) {
	// create wallet with 2 timed out dc bills
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, tempNonce, nonceBytes, 1, 0), addDcBill(t, k, tempNonce, nonceBytes, 2, 0)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nonceBytes, 0, 0, k)
	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		},
	}, am)

	// when dc runs
	counter := uint64(0)
	_, err = w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	// then a swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	txSwap := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, txSwap.TargetValue)
	require.EqualValues(t, [][]byte{nonceBytes, nonceBytes}, txSwap.BillIdentifiers)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
	require.Len(t, txSwap.DcTransfers, 2)
	require.Len(t, txSwap.Proofs, 2)

	// and expected swap is updated with swap timeout
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(nonceBytes)]
	require.EqualValues(t, nonceBytes, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, swapTimeoutBlockCount, swap.timeout)
}

func TestDcNonceHashIsCalculatedInCorrectBillOrder(t *testing.T) {
	bills := []*Bill{
		{Id: uint256.NewInt(2)},
		{Id: uint256.NewInt(1)},
		{Id: uint256.NewInt(0)},
	}
	hasher := crypto.SHA256.New()
	for i := len(bills) - 1; i >= 0; i-- {
		hasher.Write(bills[i].GetID())
	}
	expectedNonce := hasher.Sum(nil)

	nonce := calculateDcNonce(bills)
	require.EqualValues(t, expectedNonce, nonce)
}

func TestSwapTxValuesAreCalculatedInCorrectBillOrder(t *testing.T) {
	w, _ := CreateTestWallet(t, nil)
	k, _ := w.am.GetAccountKey(0)

	dcBills := []*Bill{
		{Id: uint256.NewInt(2), BlockProof: &BlockProof{Tx: moneytesttx.CreateRandomDcTx()}},
		{Id: uint256.NewInt(1), BlockProof: &BlockProof{Tx: moneytesttx.CreateRandomDcTx()}},
		{Id: uint256.NewInt(0), BlockProof: &BlockProof{Tx: moneytesttx.CreateRandomDcTx()}},
	}
	dcNonce := calculateDcNonce(dcBills)
	var dcBillIds [][]byte
	for _, dcBill := range dcBills {
		dcBillIds = append(dcBillIds, dcBill.GetID())
	}

	var protoDcBills []*bp.Bill
	for _, b := range dcBills {
		protoDcBills = append(protoDcBills, b.ToProto())
	}

	tx, err := txbuilder.CreateSwapTx(k, w.SystemID(), protoDcBills, dcNonce, dcBillIds, 10)
	require.NoError(t, err)
	swapTx := parseSwapTx(t, tx)

	// verify bill ids in swap tx are in correct order (equal hash values)
	hasher := crypto.SHA256.New()
	for _, billId := range swapTx.BillIdentifiers {
		hasher.Write(billId)
	}
	actualDcNonce := hasher.Sum(nil)
	require.EqualValues(t, dcNonce, actualDcNonce)
}

func TestSwapContainsUnconfirmedDustBillIds(t *testing.T) {
	// create wallet with three bills
	_ = log.InitStdoutLogger(log.INFO)
	b1 := addBill(1)
	b2 := addBill(2)
	b3 := addBill(3)
	nonce := calculateDcNonce([]*Bill{b1, b2, b3})
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)

	billsList := createBillListJsonResponse([]*Bill{b1, b2, b3})
	// proofs are polled twice, one for the regular bills and one for dc bills
	proofList := createBlockProofJsonResponse(t, []*Bill{b1, b2, b3}, nil, 0, dcTimeoutBlockCount, k)
	proofList = append(proofList, createBlockProofJsonResponse(t, []*Bill{addDcBill(t, k, b1.Id, nonce, 1, dcTimeoutBlockCount), addDcBill(t, k, b2.Id, nonce, 2, dcTimeoutBlockCount), addDcBill(t, k, b3.Id, nonce, 3, dcTimeoutBlockCount)}, nonce, 0, dcTimeoutBlockCount, k)...)
	w, mockClient := CreateTestWalletWithManager(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		},
	}, am)

	// when dc runs
	counter := uint64(0)
	_, err = w.collectDust(context.Background(), false, 0, &counter)
	require.NoError(t, err)

	verifyBlockHeight(t, w, 0)

	// and three dc txs are broadcast
	dcTxs := mockClient.GetRecordedTransactions()
	require.Len(t, dcTxs, 4)
	for _, tx := range dcTxs[0:3] {
		require.NotNil(t, parseDcTx(t, tx))
	}

	// and swap should contain all bill ids
	tx := mockClient.GetRecordedTransactions()[3]
	swapOrder := parseSwapTx(t, tx)
	require.EqualValues(t, nonce, tx.UnitId)
	require.Len(t, swapOrder.BillIdentifiers, 3)
	require.Equal(t, b1.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[0]))
	require.Equal(t, b2.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[1]))
	require.Equal(t, b3.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[2]))
	require.Len(t, swapOrder.DcTransfers, 3)
	require.Equal(t, dcTxs[0], swapOrder.DcTransfers[0])
	require.Equal(t, dcTxs[1], swapOrder.DcTransfers[1])
	require.Equal(t, dcTxs[2], swapOrder.DcTransfers[2])
}

func addBill(value uint64) *Bill {
	b1 := Bill{
		Id:         uint256.NewInt(value),
		Value:      value,
		TxHash:     hash.Sum256([]byte{byte(value)}),
		BlockProof: &BlockProof{},
	}
	return &b1
}

func addDcBill(t *testing.T, k *account.AccountKey, id *uint256.Int, nonce []byte, value uint64, timeout uint64) *Bill {
	b := Bill{
		Id:         id,
		Value:      value,
		TxHash:     hash.Sum256([]byte{byte(value)}),
		BlockProof: &BlockProof{},
	}

	tx, err := txbuilder.CreateDustTx(k, []byte{0, 0, 0, 0}, b.ToProto(), nonce, timeout)
	require.NoError(t, err)
	b.BlockProof = &BlockProof{Tx: tx}

	b.IsDcBill = true
	b.DcNonce = nonce
	b.DcTimeout = timeout
	b.DcExpirationTimeout = dustBillDeletionTimeout

	require.NoError(t, err)
	return &b
}

func verifyBlockHeight(t *testing.T, w *Wallet, blockHeight uint64) {
	actualBlockHeight, err := w.AlphabillClient.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, blockHeight, actualBlockHeight)
}

func verifyBalance(t *testing.T, w *Wallet, balance uint64, pubKey []byte) {
	actualBalance, err := w.restClient.GetBalance(pubKey, false)
	require.NoError(t, err)
	require.EqualValues(t, balance, actualBalance)
}

func verifyTotalBalance(t *testing.T, w *Wallet, balance uint64, pubKey []byte) {
	actualBalance, err := w.restClient.GetBalance(pubKey, true)
	require.NoError(t, err)
	require.EqualValues(t, balance, actualBalance)
}

func parseBillTransferTx(t *testing.T, tx *txsystem.Transaction) *billtx.TransferAttributes {
	btTx := &billtx.TransferAttributes{}
	err := tx.TransactionAttributes.UnmarshalTo(btTx)
	require.NoError(t, err)
	return btTx
}

func parseDcTx(t *testing.T, tx *txsystem.Transaction) *billtx.TransferDCAttributes {
	dcTx := &billtx.TransferDCAttributes{}
	err := tx.TransactionAttributes.UnmarshalTo(dcTx)
	require.NoError(t, err)
	return dcTx
}

func parseSwapTx(t *testing.T, tx *txsystem.Transaction) *billtx.SwapDCAttributes {
	txSwap := &billtx.SwapDCAttributes{}
	err := tx.TransactionAttributes.UnmarshalTo(txSwap)
	require.NoError(t, err)
	return txSwap
}
