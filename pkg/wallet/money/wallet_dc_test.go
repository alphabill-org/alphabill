package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestDustCollectionWontRunForSingleBill(t *testing.T) {
	// create wallet with a single bill
	bills := []*Bill{addBill(1)}
	billsList := createBillListJsonResponse(bills)

	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 3, customBillList: billsList}))

	// when dc runs
	err := w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// then no txs are broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 0)
}

func TestDustCollectionMaxBillCount(t *testing.T) {
	// create wallet with max allowed bills for dc
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	bills := make([]*Bill, maxBillsForDustCollection)
	for i := 0; i < maxBillsForDustCollection; i++ {
		bills[i] = addBill(uint64(i))
	}
	billsList := createBillListResponse(bills)
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	billListCallFlag := false
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			if billListCallFlag {
				return createBillListResponse([]*Bill{addBill(uint64(maxBillsForDustCollection + 1))}), nil
			}
			billListCallFlag = true
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nonceBytes, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// then dc total tx count should be equal to the maximum allowed bills plus 1 for the swap
	require.Len(t, recordedTxs, maxBillsForDustCollection+1)
}

func TestDustCollectionMaxBillCountOverLimit(t *testing.T) {
	// create wallet with max allowed bills for dc + 1
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	bills := make([]*Bill, maxBillsForDustCollection+1)
	for i := 0; i < maxBillsForDustCollection+1; i++ {
		bills[i] = addBill(uint64(i))
	}
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	billListCounter := 0
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			// simulate progress during multiple calls for dc processing:
			switch billListCounter {
			case 0:
				// First call: 100 bills
				billListCounter++
				return createBillListResponse(bills), nil
			case 1:
				// Second call: 1 original bill + 1 new bill (simulate swap)
				bills = append(bills, addBill(uint64(maxBillsForDustCollection+1)))
				billListCounter++
				return createBillListResponse(bills[maxBillsForDustCollection:]), nil
			case 2:
				// Third call: 1 new bill (final swap)
				bills = append(bills, addBill(uint64(maxBillsForDustCollection+2)))
				return createBillListResponse(bills[maxBillsForDustCollection+2:]), nil
			}
			return nil, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nonceBytes, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// then dc total tx count should be equal to the maximum allowed bills plus 4:
	// - First submission: max dc + 1 swap
	//   after this, 2 bills remain (1 that was over the max limit and not collected, and 1 new bill from the swap)
	// - Second submission: 2 dc + 1 swap
	require.Len(t, recordedTxs, maxBillsForDustCollection+4)
}

func TestBasicDustCollection(t *testing.T) {
	// create wallet with 2 normal bills
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addBill(1), addBill(2)}
	billsList := createBillListResponse(bills)
	expectedDcNonce := calculateDcNonce(bills)
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	recordedSwaps := make(map[string]*types.TransactionOrder, 0)
	recordedDCs := make(map[string]*types.TransactionOrder, 0)
	billListCallFlag := false
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			if billListCallFlag {
				return createBillListResponse([]*Bill{addBill(3)}), nil
			}
			billListCallFlag = true
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nonceBytes, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
				if tx.PayloadType() == "transDC" {
					recordedDCs[string(tx.UnitID())] = tx
				}
				if tx.PayloadType() == "swapDC" {
					recordedSwaps[string(tx.UnitID())] = tx
				}
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// then two dc txs are broadcast plus one swap
	require.Len(t, recordedTxs, 3)
	for _, tx := range recordedDCs {
		dcTx := parseDcTx(t, tx)
		require.NotNil(t, dcTx)
		var bill *Bill
		for _, b := range bills {
			if bytes.Equal(tx.UnitID(), util.Uint256ToBytes(b.Id)) {
				bill = b
				break
			}
		}
		require.EqualValues(t, expectedDcNonce, dcTx.Nonce)
		require.EqualValues(t, bill.Value, dcTx.TargetValue)
		require.EqualValues(t, bill.TxHash, dcTx.Backlink)
		require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), dcTx.TargetBearer)
	}
}

func TestDustCollectionWithSwap(t *testing.T) {
	// create wallet with 2 normal bills
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addBill(1), addBill(2)}
	expectedDcNonce := calculateDcNonce(bills)
	billsList := createBillListResponse(bills)
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	recordedSwaps := make(map[string]*types.TransactionOrder, 0)
	recordedDCs := make(map[string]*types.TransactionOrder, 0)
	billListCallFlag := false
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			if billListCallFlag {
				return createBillListResponse([]*Bill{addBill(3)}), nil
			}
			billListCallFlag = true
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, expectedDcNonce, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
				if tx.PayloadType() == "transDC" {
					recordedDCs[string(tx.UnitID())] = tx
				}
				if tx.PayloadType() == "swapDC" {
					recordedSwaps[string(tx.UnitID())] = tx
				}
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// then two dc txs + one swap tx are broadcast
	require.Len(t, recordedTxs, 3)
	for _, tx := range recordedDCs {
		require.NotNil(t, parseDcTx(t, tx))
	}
	for _, tx := range recordedSwaps {
		txSwap := parseSwapTx(t, tx)
		require.EqualValues(t, 3, txSwap.TargetValue)
		require.EqualValues(t, [][]byte{util.Uint256ToBytes(bills[0].Id), util.Uint256ToBytes(bills[1].Id)}, txSwap.BillIdentifiers)
		require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
		require.Len(t, txSwap.DcTransfers, 2)
		require.Len(t, txSwap.Proofs, 2)
	}
}

func TestSwapWithExistingDCBillsBeforeDCTimeout(t *testing.T) {
	// create wallet with 2 dc bills
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	ids := [][]byte{util.Uint256ToBytes(uint256.NewInt(1)), util.Uint256ToBytes(uint256.NewInt(2))}
	bills := []*Bill{addDcBill(t, k, uint256.NewInt(1), nonceBytes, ids, 1, dcTimeoutBlockCount, 3), addDcBill(t, k, uint256.NewInt(2), nonceBytes, ids, 2, dcTimeoutBlockCount, 3)}
	billsList := createBillListResponse(bills)
	expectedDcNonce := calculateDcNonce(bills)
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	roundNr := uint64(5)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			roundNr++
			return roundNr, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, expectedDcNonce, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, mockClient := CreateTestWalletWithManager(t, backendMock, am)

	// set specific round number
	mockClient.SetMaxRoundNumber(roundNr)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// then a swap tx is broadcast
	require.Len(t, recordedTxs, 1)
	for _, tx := range recordedTxs {
		txSwap := parseSwapTx(t, tx)
		require.EqualValues(t, 3, txSwap.TargetValue)
		require.EqualValues(t, [][]byte{util.Uint256ToBytes(bills[0].Id), util.Uint256ToBytes(bills[1].Id)}, txSwap.BillIdentifiers)
		require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
		require.Len(t, txSwap.DcTransfers, 2)
		require.Len(t, txSwap.Proofs, 2)
	}
}

func TestSwapWithExistingExpiredDCBills(t *testing.T) {
	// create wallet with 2 timed out dc bills
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	ids := [][]byte{util.Uint256ToBytes(uint256.NewInt(1)), util.Uint256ToBytes(uint256.NewInt(2))}
	bills := []*Bill{addDcBill(t, k, uint256.NewInt(1), nonceBytes, ids, 1, 0, 3), addDcBill(t, k, uint256.NewInt(2), nonceBytes, ids, 2, 0, 3)}
	billsList := createBillListResponse(bills)
	expectedDcNonce := calculateDcNonce(bills)
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, expectedDcNonce, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// then a swap tx is broadcast
	require.Len(t, recordedTxs, 1)
	for _, tx := range recordedTxs {
		txSwap := parseSwapTx(t, tx)
		require.EqualValues(t, 3, txSwap.TargetValue)
		require.EqualValues(t, [][]byte{util.Uint256ToBytes(bills[0].Id), util.Uint256ToBytes(bills[1].Id)}, txSwap.BillIdentifiers)
		require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
		require.Len(t, txSwap.DcTransfers, 2)
		require.Len(t, txSwap.Proofs, 2)
	}
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
		{Id: uint256.NewInt(2), TxProof: &wallet.Proof{TxRecord: createRandomDcTx()}},
		{Id: uint256.NewInt(1), TxProof: &wallet.Proof{TxRecord: createRandomDcTx()}},
		{Id: uint256.NewInt(0), TxProof: &wallet.Proof{TxRecord: createRandomDcTx()}},
	}
	dcNonce := calculateDcNonce(dcBills)
	var dcBillIds [][]byte
	for _, dcBill := range dcBills {
		dcBillIds = append(dcBillIds, dcBill.GetID())
	}

	var protoDcBills []*wallet.Bill
	for _, b := range dcBills {
		protoDcBills = append(protoDcBills, b.ToGenericBill())
	}

	swapTxOrder, err := txbuilder.NewSwapTx(k, w.SystemID(), protoDcBills, dcNonce, dcBillIds, 10)
	require.NoError(t, err)

	swapAttr := &billtx.SwapDCAttributes{}
	err = swapTxOrder.UnmarshalAttributes(swapAttr)
	require.NoError(t, err)

	// verify bill ids in swap tx are in correct order (equal hash values)
	hasher := crypto.SHA256.New()
	for _, billId := range swapAttr.BillIdentifiers {
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
	bills := []*Bill{b1, b2, b3}
	nonce := calculateDcNonce(bills)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	billsList := createBillListResponse(bills)
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	recordedSwaps := make(map[string]*types.TransactionOrder, 0)
	recordedDCs := make(map[string]*types.TransactionOrder, 0)
	billListCallFlag := false
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			if billListCallFlag {
				return createBillListResponse([]*Bill{addBill(4)}), nil
			}
			billListCallFlag = true
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nil, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
				if tx.PayloadType() == "transDC" {
					recordedDCs[string(tx.UnitID())] = tx
				}
				if tx.PayloadType() == "swapDC" {
					recordedSwaps[string(tx.UnitID())] = tx
				}
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// and three dc txs are broadcast
	require.Len(t, recordedTxs, 4)

	// and swap should contain all bill ids
	for _, tx := range recordedSwaps {
		swapOrder := parseSwapTx(t, tx)
		require.EqualValues(t, nonce, tx.UnitID())
		require.Len(t, swapOrder.BillIdentifiers, 3)
		require.Equal(t, b1.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[0]))
		require.Equal(t, b2.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[1]))
		require.Equal(t, b3.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[2]))
		require.Len(t, swapOrder.DcTransfers, 3)
		require.NotNil(t, recordedDCs[string(swapOrder.DcTransfers[0].TransactionOrder.UnitID())])
		require.NotNil(t, recordedDCs[string(swapOrder.DcTransfers[1].TransactionOrder.UnitID())])
		require.NotNil(t, recordedDCs[string(swapOrder.DcTransfers[2].TransactionOrder.UnitID())])
	}
}

func TestBlockingDcWithNormalBills(t *testing.T) {
	// wallet contains 2 normal bills
	bills := []*Bill{addBill(1), addBill(2)}
	nonce := calculateDcNonce(bills)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	billsList := createBillListResponse(bills)
	billIds, dcSum := getBillIdsAndSum(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	billListCallFlag := false
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			if billListCallFlag {
				return createBillListResponse([]*Bill{addBill(3)}), nil
			}
			billListCallFlag = true
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nil, billIds, dcTimeoutBlockCount, dcSum, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// and dc + swap txs should be sent
	require.Len(t, recordedTxs, 3)
	for _, tx := range recordedTxs {
		if tx.PayloadType() == "transDC" {
			dcTx := parseDcTx(t, tx)
			require.EqualValues(t, nonce, dcTx.Nonce)
		} else {
			swapTx := parseSwapTx(t, tx)
			require.EqualValues(t, 3, swapTx.TargetValue)
		}
	}
}

func TestBlockingDCWithExistingExpiredDCBills(t *testing.T) {
	// create wallet with 2 timed out dc bills
	tempNonce := uint256.NewInt(1)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	ids := [][]byte{util.Uint256ToBytes(uint256.NewInt(1)), util.Uint256ToBytes(uint256.NewInt(2))}
	bills := []*Bill{addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(tempNonce), ids, 1, 0, 3), addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(tempNonce), ids, 2, 0, 3)}
	billsList := createBillListResponse(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, util.Uint256ToBytes(tempNonce), ids, dcTimeoutBlockCount, 3, nil), nil
				}
			}
			return nil, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dc runs
	err = w.collectDust(context.Background(), 0)
	require.NoError(t, err)

	// and swap tx should be sent
	for _, tx := range recordedTxs {
		if tx.PayloadType() == "swapDC" {
			swapTx := parseSwapTx(t, tx)
			require.EqualValues(t, 3, swapTx.TargetValue)
		}
	}
}

func addBill(value uint64) *Bill {
	b1 := Bill{
		Id:      uint256.NewInt(value),
		Value:   value,
		TxHash:  hash.Sum256([]byte{byte(value)}),
		TxProof: &wallet.Proof{},
	}
	return &b1
}

func addDcBill(t *testing.T, k *account.AccountKey, id *uint256.Int, nonce []byte, billIds [][]byte, value, timeout, dcSum uint64) *Bill {
	b := Bill{
		Id:      id,
		Value:   value,
		TxHash:  hash.Sum256([]byte{byte(value)}),
		TxProof: &wallet.Proof{},
	}

	tx, err := txbuilder.NewDustTx(k, []byte{0, 0, 0, 0}, b.ToGenericBill(), nonce, billIds, timeout, dcSum)
	require.NoError(t, err)
	b.TxProof = &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx}}

	b.IsDcBill = true
	b.DcNonce = nonce
	b.DcTimeout = timeout
	b.DcExpirationTimeout = dustBillDeletionTimeout

	require.NoError(t, err)
	return &b
}

func parseBillTransferTx(t *testing.T, tx *types.TransactionOrder) *billtx.TransferAttributes {
	transferTx := &billtx.TransferAttributes{}
	err := tx.UnmarshalAttributes(transferTx)
	require.NoError(t, err)
	return transferTx
}

func parseDcTx(t *testing.T, tx *types.TransactionOrder) *billtx.TransferDCAttributes {
	dcTx := &billtx.TransferDCAttributes{}
	err := tx.UnmarshalAttributes(dcTx)
	require.NoError(t, err)
	return dcTx
}

func parseSwapTx(t *testing.T, tx *types.TransactionOrder) *billtx.SwapDCAttributes {
	txSwap := &billtx.SwapDCAttributes{}
	err := tx.UnmarshalAttributes(txSwap)
	require.NoError(t, err)
	return txSwap
}

func createRandomDcTx() *types.TransactionRecord {
	return &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       []byte{0, 0, 0, 0},
				Type:           billtx.PayloadTypeTransDC,
				UnitID:         hash.Sum256([]byte{0x00}),
				Attributes:     randomTransferDCAttributes(),
				ClientMetadata: &types.ClientMetadata{Timeout: 1000},
			},
			OwnerProof: script.PredicateArgumentEmpty(),
		},
		ServerMetadata: nil,
	}
}

func randomTransferDCAttributes() []byte {
	attr := &billtx.TransferDCAttributes{
		TargetBearer: script.PredicateAlwaysTrue(),
		Backlink:     hash.Sum256([]byte{}),
		Nonce:        hash.Sum256([]byte{}),
		TargetValue:  100,
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}
