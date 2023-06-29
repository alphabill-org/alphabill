package money

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	beclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type (
	backendMockReturnConf struct {
		balance                  uint64
		roundNumber              uint64
		billId                   *uint256.Int
		billValue                uint64
		billTxHash               string
		proofList                []string
		customBillList           string
		customPath               string
		customFullPath           string
		customResponse           string
		feeCreditBill            *wallet.Bill
		postTransactionsResponse interface{}
	}
)

func CreateTestWallet(t *testing.T, backend BackendAPI) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)

	return CreateTestWalletWithManager(t, backend, am)
}

func CreateTestWalletWithManager(t *testing.T, backend BackendAPI, am account.Manager) (*Wallet, *clientmock.MockAlphabillClient) {
	err := CreateNewWallet(am, "")
	require.NoError(t, err)

	mockClient := clientmock.NewMockAlphabillClient(clientmock.WithMaxBlockNumber(0), clientmock.WithBlocks(map[uint64]*types.Block{}))
	w, err := LoadExistingWallet(am, backend)
	require.NoError(t, err)
	return w, mockClient
}

func withBackendMock(t *testing.T, br *backendMockReturnConf) BackendAPI {
	_, serverAddr := mockBackendCalls(br)
	restClient, err := beclient.New(serverAddr.Host)
	require.NoError(t, err)
	return restClient
}

func CreateTestWalletFromSeed(t *testing.T, br *backendMockReturnConf) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, testMnemonic)
	require.NoError(t, err)

	mockClient := &clientmock.MockAlphabillClient{}
	_, serverAddr := mockBackendCalls(br)
	restClient, err := beclient.New(serverAddr.Host)
	require.NoError(t, err)
	w, err := LoadExistingWallet(am, restClient)
	require.NoError(t, err)
	return w, mockClient
}

func mockBackendCalls(br *backendMockReturnConf) (*httptest.Server, *url.URL) {
	proofCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.customPath || r.URL.RequestURI() == br.customFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.customResponse))
		} else {
			path := r.URL.Path
			switch {
			case path == "/"+beclient.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.balance)))
			case path == "/"+beclient.RoundNumberPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"roundNumber": "%d"}`, br.roundNumber)))
			case path == "/"+beclient.ProofPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(br.proofList[proofCount%len(br.proofList)]))
				proofCount++
			case path == "/"+beclient.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"total": 1, "bills": [{"id":"%s","value":"%d","txHash":"%s","isDcBill":false}]}`, toBillId(br.billId), br.billValue, br.billTxHash)))
				}
			case strings.Contains(path, beclient.FeeCreditPath):
				w.WriteHeader(http.StatusOK)
				fcb, _ := json.Marshal(br.feeCreditBill)
				w.Write(fcb)
			case strings.Contains(path, beclient.TransactionsPath):
				if br.postTransactionsResponse == nil {
					w.WriteHeader(http.StatusAccepted)
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					res, _ := json.Marshal(br.postTransactionsResponse)
					w.Write(res)
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func toBillId(i *uint256.Int) string {
	return base64.StdEncoding.EncodeToString(util.Uint256ToBytes(i))
}

func createBlockProofResponse(t *testing.T, b *Bill, overrideNonce []byte, timeout uint64, k *account.AccountKey) *wallet.Proof {
	w, mockClient := CreateTestWallet(t, nil)
	if k == nil {
		k, _ = w.am.GetAccountKey(0)
	}
	var dcTx *types.TransactionOrder
	if overrideNonce != nil {
		dcTx, _ = txbuilder.NewDustTx(k, w.SystemID(), b.ToGenericBillProof().Bill, overrideNonce, timeout)
	} else {
		dcTx, _ = txbuilder.NewDustTx(k, w.SystemID(), b.ToGenericBillProof().Bill, calculateDcNonce([]*Bill{b}), timeout)
	}
	txRecord := &types.TransactionRecord{TransactionOrder: dcTx}
	mockClient.SetBlock(&types.Block{
		Header: &types.Header{
			SystemID:          w.SystemID(),
			PreviousBlockHash: hash.Sum256([]byte{}),
		},
		Transactions:       []*types.TransactionRecord{txRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: timeout}},
	})
	txProof := &wallet.Proof{
		TxRecord: txRecord,
		TxProof: &types.TxProof{
			BlockHeaderHash:    []byte{0},
			Chain:              []*types.GenericChainItem{{Hash: []byte{0}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: timeout}},
		},
	}
	return txProof
}

func createBillListResponse(bills []*Bill, dcMetadata map[string]*backend.DCMetadata) *backend.ListBillsResponse {
	billVMs := make([]*wallet.Bill, len(bills))
	for i, b := range bills {
		billVMs[i] = &wallet.Bill{
			Id:      b.GetID(),
			Value:   b.Value,
			TxHash:  b.TxHash,
			DcNonce: b.DcNonce,
		}
	}
	return &backend.ListBillsResponse{Bills: billVMs, Total: len(bills), DCMetadata: dcMetadata}
}

func createBillListJsonResponse(bills []*Bill) string {
	billsResponse := createBillListResponse(bills, nil)
	res, _ := json.Marshal(billsResponse)
	return string(res)
}

type backendAPIMock struct {
	getBalance       func(pubKey []byte, includeDCBills bool) (uint64, error)
	listBills        func(pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error)
	getBills         func(pubKey []byte) ([]*wallet.Bill, error)
	getRoundNumber   func() (uint64, error)
	getTxProof       func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	getFeeCreditBill func(ctx context.Context, unitID []byte) (*wallet.Bill, error)
	postTransactions func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
}

func (b *backendAPIMock) GetBills(pubKey []byte) ([]*wallet.Bill, error) {
	if b.getBills != nil {
		return b.getBills(pubKey)
	}
	return nil, errors.New("getBills not implemented")
}

func (b *backendAPIMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if b.getRoundNumber != nil {
		return b.getRoundNumber()
	}
	return 0, errors.New("getRoundNumber not implemented")
}

func (b *backendAPIMock) GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error) {
	if b.getFeeCreditBill != nil {
		return b.getFeeCreditBill(ctx, unitID)
	}
	return nil, errors.New("getFeeCreditBill not implemented")
}

func (b *backendAPIMock) GetBalance(pubKey []byte, includeDCBills bool) (uint64, error) {
	if b.getBalance != nil {
		return b.getBalance(pubKey, includeDCBills)
	}
	return 0, errors.New("getBalance not implemented")
}

func (b *backendAPIMock) ListBills(pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error) {
	if b.listBills != nil {
		return b.listBills(pubKey, includeDCBills, includeDCMetadata)
	}
	return nil, errors.New("listBills not implemented")
}

func (b *backendAPIMock) GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	if b.getTxProof != nil {
		return b.getTxProof(ctx, unitID, txHash)
	}
	return nil, errors.New("getTxProof not implemented")
}

func (b *backendAPIMock) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
	if b.postTransactions != nil {
		return b.postTransactions(ctx, pubKey, txs)
	}
	return nil
}
