package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

const pubKeyHex = "0x038003e218eea360cbf580ebb90cc8c8caf0ccef4bf660ea9ab4fc06b5c367b038"
const billId = "MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg="

func TestGetBalance(t *testing.T) {
	mockServer, mockAddress := mockGetBalanceCall(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := New(mockAddress.Host)
	require.NoError(t, err)

	balance, err := restClient.GetBalance(context.Background(), pubKey, true)
	require.NoError(t, err)
	require.EqualValues(t, 15, balance)
}

func TestListBills(t *testing.T) {
	mockServer, mockAddress := mockListBillsCall(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := New(mockAddress.Host)
	require.NoError(t, err)

	billsResponse, err := restClient.ListBills(context.Background(), pubKey, true, "", defaultPagingLimit)
	require.NoError(t, err)
	require.Len(t, billsResponse.Bills, 8)
	b, _ := base64.StdEncoding.DecodeString(billId)
	require.EqualValues(t, b, billsResponse.Bills[0].Id)
}

func TestListBillsWithPaging(t *testing.T) {
	mockServer, mockAddress := mockListBillsCallWithPaging(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := New(mockAddress.Host)
	require.NoError(t, err)

	billsResponse, err := restClient.ListBills(context.Background(), pubKey, true, "", defaultPagingLimit)
	require.NoError(t, err)
	require.Len(t, billsResponse.Bills, 5)
	b, _ := base64.StdEncoding.DecodeString(billId)
	require.EqualValues(t, b, billsResponse.Bills[0].Id)
}

func TestTxHistoryWithPaging(t *testing.T) {
	limit := 2
	offset := "foo"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/"+TxHistoryPath) {
			t.Errorf("Expected to request '%v', got: %s", TxHistoryPath, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		} else {
			require.Equal(t, r.URL.Query().Get(sdk.QueryParamOffsetKey), offset)
			require.Equal(t, r.URL.Query().Get(sdk.QueryParamLimit), strconv.Itoa(limit))
			w.Header().Set(sdk.ContentType, sdk.ApplicationCbor)
			w.WriteHeader(http.StatusOK)
			res := []*sdk.TxHistoryRecord{
				{
					TxHash: test.RandomBytes(32),
					UnitID: test.RandomBytes(32),
					Kind:   sdk.INCOMING,
				},
				{
					TxHash: test.RandomBytes(32),
					UnitID: test.RandomBytes(32),
					Kind:   sdk.OUTGOING,
				},
			}
			bytes, err := cbor.Marshal(res)
			require.NoError(t, err)
			_, _ = w.Write(bytes)
		}
	}))

	serverAddress, _ := url.Parse(server.URL)

	defer server.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := New(serverAddress.Host)
	require.NoError(t, err)

	historyResponse, offset, err := restClient.GetTxHistory(context.Background(), pubKey, offset, limit)
	require.NoError(t, err)
	require.Equal(t, "", offset)
	require.Len(t, historyResponse, 2)
}

func TestGetTxProof(t *testing.T) {
	mockServer, mockAddress := mockGetTxProofCall(t)
	defer mockServer.Close()

	restClient, _ := New(mockAddress.Host)
	proofResponse, err := restClient.GetTxProof(context.Background(), []byte{0x00}, []byte{0x01})

	require.NoError(t, err)
	require.NotNil(t, proofResponse)
}

func TestGetTxProof_404_UrlOK(t *testing.T) {
	mockAddr := mockNotFoundErrorResponse(t, "no proof found for tx")
	restClient, err := New(mockAddr.Host)
	require.NoError(t, err)

	res, err := restClient.GetTxProof(context.Background(), []byte{0x00}, []byte{0x01})
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestGetTxProof_404_UrlNOK(t *testing.T) {
	mockAddr := mockNotFoundResponse(t)
	restClient, err := New(mockAddr.Host)
	require.NoError(t, err)

	res, err := restClient.GetTxProof(context.Background(), []byte{0x00}, []byte{0x01})
	require.ErrorContains(t, err, "failed to decode error from the response body")
	require.Nil(t, res)
}

func TestBlockHeight(t *testing.T) {
	mockServer, mockAddress := mockGetBlockHeightCall(t)
	defer mockServer.Close()

	restClient, _ := New(mockAddress.Host)
	blockHeight, err := restClient.GetRoundNumber(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 1000, blockHeight)
}

func Test_NewClient(t *testing.T) {
	t.Run("invalid URL", func(t *testing.T) {
		mbc, err := New("x:y:z")
		require.ErrorContains(t, err, "error parsing Money Backend Client base URL")
		require.Nil(t, mbc)
	})

	t.Run("valid URL", func(t *testing.T) {
		cases := []struct{ param, url string }{
			{param: "127.0.0.1", url: "http://127.0.0.1"},
			{param: "127.0.0.1:8000", url: "http://127.0.0.1:8000"},
			{param: "http://127.0.0.1", url: "http://127.0.0.1"},
			{param: "http://127.0.0.1:8080", url: "http://127.0.0.1:8080"},
			{param: "https://127.0.0.1", url: "https://127.0.0.1"},
			{param: "https://127.0.0.1:8080", url: "https://127.0.0.1:8080"},
			{param: "ab-dev.guardtime.com", url: "http://ab-dev.guardtime.com"},
			{param: "https://ab-dev.guardtime.com", url: "https://ab-dev.guardtime.com"},
			{param: "ab-dev.guardtime.com:7777", url: "http://ab-dev.guardtime.com:7777"},
			{param: "https://ab-dev.guardtime.com:8888", url: "https://ab-dev.guardtime.com:8888"},
		}

		for _, tc := range cases {
			mbc, err := New(tc.param)
			if err != nil {
				t.Errorf("unexpected error for parameter %q: %v", tc.param, err)
			}
			if mbc.BaseUrl.String() != tc.url {
				t.Errorf("expected URL for %q to be %q, got %q", tc.param, tc.url, mbc.BaseUrl)
			}
		}
	})
}

func TestGetFeeCreditBill(t *testing.T) {
	serverURL := mockGetFeeCreditBillCall(t)
	restClient, _ := New(serverURL.Host)
	response, err := restClient.GetFeeCreditBill(context.Background(), []byte{})
	require.NoError(t, err)

	expectedBillID, _ := base64.StdEncoding.DecodeString(billId)
	require.EqualValues(t, expectedBillID, response.Id)
}

func TestGetFeeCreditBill_404_UrlOK(t *testing.T) {
	mockAddr := mockNotFoundErrorResponse(t, "fee credit bill does not exist")
	restClient, err := New(mockAddr.Host)
	require.NoError(t, err)

	res, err := restClient.GetFeeCreditBill(context.Background(), []byte{})
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestGetFeeCreditBill_404_UrlNOK(t *testing.T) {
	mockAddr := mockNotFoundResponse(t)
	restClient, err := New(mockAddr.Host)
	require.NoError(t, err)

	res, err := restClient.GetFeeCreditBill(context.Background(), []byte{})
	require.ErrorContains(t, err, "failed to decode error from the response body")
	require.Nil(t, res)
}

func TestPostTransactions(t *testing.T) {
	mockServer, mockAddress := mockPostTransactionsCall(t, http.StatusAccepted, "")
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := New(mockAddress.Host)
	require.NoError(t, err)

	txs := &sdk.Transactions{Transactions: []*types.TransactionOrder{
		testtransaction.NewTransactionOrder(t),
		testtransaction.NewTransactionOrder(t),
		testtransaction.NewTransactionOrder(t),
	}}
	err = restClient.PostTransactions(context.Background(), pubKey, txs)
	require.NoError(t, err)
}

func TestPostTransactionsError(t *testing.T) {
	errMsg := `{"00000000c4f0a6c28423da2fbe739a0a46ae437ce670eb6ba5fcc3524568d9a1":"transaction has timed out: transaction timeout round is 1905, current round is 1906"}`
	mockServer, mockAddress := mockPostTransactionsCall(t, http.StatusAccepted, errMsg)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := New(mockAddress.Host)
	require.NoError(t, err)

	txs := &sdk.Transactions{Transactions: []*types.TransactionOrder{
		testtransaction.NewTransactionOrder(t),
		testtransaction.NewTransactionOrder(t),
		testtransaction.NewTransactionOrder(t),
	}}
	err = restClient.PostTransactions(context.Background(), pubKey, txs)
	require.ErrorContains(t, err, "failed to process some of the transactions:\n"+
		"00000000c4f0a6c28423da2fbe739a0a46ae437ce670eb6ba5fcc3524568d9a1: transaction has timed out: transaction timeout round is 1905, current round is 1906")
}

func TestGetInfo(t *testing.T) {
	mockServer, mockAddress := mockGetInfoRequest(t)
	defer mockServer.Close()

	restClient, err := New(mockAddress.Host)
	require.NoError(t, err)

	infoResponse, err := restClient.GetInfo(context.Background())
	require.NoError(t, err)
	require.NotNil(t, infoResponse)
	require.Equal(t, "00000000", infoResponse.SystemID)
	require.Equal(t, "money backend", infoResponse.Name)
}

func mockGetBalanceCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + BalancePath) {
			t.Errorf("Expected to request '%v', got: %s", BalancePath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"balance": "15"}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockListBillsCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + ListBillsPath) {
			t.Errorf("Expected to request '%v', got: %s", ListBillsPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDcBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDcBill":false}]}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockListBillsCallWithPaging(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + ListBillsPath) {
			t.Errorf("Expected to request '%v', got: %s", ListBillsPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		if !r.URL.Query().Has(sdk.QueryParamOffsetKey) {
			w.Write([]byte(`{"bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false}]}`))
		} else if r.URL.Query().Get(sdk.QueryParamOffsetKey) == "5" {
			w.Write([]byte(`{"bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false}]}`))
		} else if r.URL.Query().Get(sdk.QueryParamOffsetKey) == "10" {
			w.Write([]byte(`{"bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false}]}`))
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetTxProofCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/units/") {
			t.Errorf("Expected to request '%v', got: %s", UnitsPath, r.URL.Path)
		}
		w.Header().Set(sdk.ContentType, sdk.ApplicationCbor)
		w.WriteHeader(http.StatusOK)
		proof := &sdk.Proof{TxRecord: nil, TxProof: nil}
		data, _ := cbor.Marshal(proof)
		w.Write(data)
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetBlockHeightCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + RoundNumberPath) {
			t.Errorf("Expected to request '%v', got: %s", RoundNumberPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"roundNumber": "1000"}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetFeeCreditBillCall(t *testing.T) *url.URL {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/"+FeeCreditPath) {
			t.Errorf("Expected to request '%v', got: %s", UnitsPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(getFeeCreditBillJsonBytes())
	}))
	t.Cleanup(server.Close)
	serverURL, _ := url.Parse(server.URL)
	return serverURL
}

func mockNotFoundErrorResponse(t *testing.T, message string) *url.URL {
	res := &sdk.ErrorResponse{
		Message: message,
	}
	resJson, err := json.Marshal(res)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(resJson)
	}))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return serverURL
}

func mockNotFoundResponse(t *testing.T) *url.URL {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return serverURL
}

func mockPostTransactionsCall(t *testing.T, statusCode int, responseBody string) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + TransactionsPath + "/" + pubKeyHex) {
			t.Errorf("Expected to request '%v', got: %s", TransactionsPath, r.URL.Path)
		}
		w.Header().Set(sdk.ContentType, sdk.ApplicationJson)
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetInfoRequest(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + InfoPath) {
			t.Errorf("Expected to request '%v', got: %s", InfoPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"system_id": "00000000", "name": "money backend"}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func getFeeCreditBillJsonBytes() []byte {
	unitID, _ := base64.StdEncoding.DecodeString(billId)
	res := &sdk.Bill{
		Id:     unitID,
		Value:  10,
		TxHash: []byte{1},
	}
	jsonBytes, _ := json.Marshal(res)
	return jsonBytes
}
