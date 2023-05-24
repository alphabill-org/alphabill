package money

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
)

func TestDcJobWithExistingDcBills(t *testing.T) {
	// wallet contains 2 dc bills with the same nonce that have timed out
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{
		addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(uint256.NewInt(1)), 1, dcTimeoutBlockCount),
		addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(uint256.NewInt(1)), 2, dcTimeoutBlockCount),
	}
	nonce := calculateDcNonce(bills)
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nonce, 0, dcTimeoutBlockCount, k)
	proofList = append(proofList, createBlockProofJsonResponse(t, bills, nonce, 0, dcTimeoutBlockCount, k)...)
	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &types.TxProof{},
		},
	}), am)
	mockClient.SetMaxBlockNumber(100)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	tx := mockClient.GetRecordedTransactions()[0]
	txSwap := parseSwapTx(t, tx)

	txo := &types.TransactionOrder{}
	err = cbor.Unmarshal(tx.Order, tx)
	require.NoError(t, err)

	// and verify each dc tx id = nonce = swap.id
	require.Len(t, txSwap.DcTransfers, 2)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTxAttr(t, txSwap.DcTransfers[i].TransactionOrder)
		require.EqualValues(t, nonce, dcTx.Nonce)
		require.EqualValues(t, nonce, txo.UnitID())
	}
}

func TestDcJobWithExistingDcAndNonDcBills(t *testing.T) {
	// wallet contains timed out dc bill and normal bill
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bill := addBill(1)
	dc := addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(uint256.NewInt(1)), 2, dcTimeoutBlockCount)
	nonce := calculateDcNonce([]*Bill{bill, dc})
	billsList := createBillListJsonResponse([]*Bill{bill, dc})
	proofList := createBlockProofJsonResponse(t, []*Bill{bill, dc}, nonce, 0, dcTimeoutBlockCount, k)

	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &types.TxProof{},
		},
	}), am)
	mockClient.SetMaxBlockNumber(100)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then swap tx is sent for the timed out dc bill
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	tx := mockClient.GetRecordedTransactions()[0]
	txSwap := parseSwapTx(t, tx)

	txo := &types.TransactionOrder{}
	err = cbor.Unmarshal(tx.Order, tx)
	require.NoError(t, err)

	// and verify nonce = swap.id = dc tx id
	require.Len(t, txSwap.DcTransfers, 1)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTxAttr(t, txSwap.DcTransfers[i].TransactionOrder)
		require.EqualValues(t, nonce, dcTx.Nonce)
		require.EqualValues(t, nonce, txo.UnitID())
	}
}

func TestDcJobWithExistingNonDcBills(t *testing.T) {
	// wallet contains 2 non dc bills
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	dcBills := []*Bill{addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(uint256.NewInt(1)), 1, dcTimeoutBlockCount), addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(uint256.NewInt(1)), 2, dcTimeoutBlockCount)}
	nonce := calculateDcNonce(dcBills)
	bills := []*Bill{addBill(1), addBill(2)}
	billsList := createBillListJsonResponse(bills)

	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonce, 0, dcTimeoutBlockCount, k)...)
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &types.TxProof{},
		}}))
	mockClient.SetMaxBlockNumber(100)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then dust txs are broadcast (plus swap)
	require.Len(t, mockClient.GetRecordedTransactions(), 3)

	// and nonces are equal
	dcTx0 := parseDcTx(t, mockClient.GetRecordedTransactions()[0])
	dcTx1 := parseDcTx(t, mockClient.GetRecordedTransactions()[1])
	require.EqualValues(t, dcTx0.Nonce, dcTx1.Nonce)
}

func TestDcJobSendsSwapsIfDcBillTimeoutHasBeenReached(t *testing.T) {
	// wallet contains 2 dc bills that both have timed out
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(uint256.NewInt(1)), 1, dcTimeoutBlockCount), addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(uint256.NewInt(1)), 2, dcTimeoutBlockCount)}
	nonce := calculateDcNonce(bills)
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nonce, 0, dcTimeoutBlockCount, k)
	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &bp.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &types.TxProof{},
		},
	}), am)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then 2 swap txs must be broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	for _, tx := range mockClient.GetRecordedTransactions() {
		require.NotNil(t, parseSwapTx(t, tx))
	}
}
