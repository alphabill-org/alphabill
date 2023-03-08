package client

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

const pubKeyHex = "0x038003e218eea360cbf580ebb90cc8c8caf0ccef4bf660ea9ab4fc06b5c367b038"
const billId = "MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg="

func TestGetBalance(t *testing.T) {
	mockServer, mockAddress := mockGetBalanceCall(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := NewClient(mockAddress.Host)
	require.NoError(t, err)

	balance, err := restClient.GetBalance(pubKey, true)
	require.NoError(t, err)
	require.EqualValues(t, 15, balance)
}

func TestListBills(t *testing.T) {
	mockServer, mockAddress := mockListBillsCall(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := NewClient(mockAddress.Host)
	require.NoError(t, err)

	billsResponse, err := restClient.ListBills(pubKey)
	require.NoError(t, err)
	require.Len(t, billsResponse.Bills, 8)
	require.EqualValues(t, 8, billsResponse.Total)
	b, _ := base64.StdEncoding.DecodeString(billId)
	require.EqualValues(t, b, billsResponse.Bills[0].Id)
}

func TestListBillsWithPaging(t *testing.T) {
	mockServer, mockAddress := mockListBillsCallWithPaging(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	require.NoError(t, err)
	restClient, err := NewClient(mockAddress.Host)
	require.NoError(t, err)

	billsResponse, err := restClient.ListBills(pubKey)
	require.NoError(t, err)
	require.Len(t, billsResponse.Bills, 13)
	require.EqualValues(t, 13, billsResponse.Total)
	b, _ := base64.StdEncoding.DecodeString(billId)
	require.EqualValues(t, b, billsResponse.Bills[0].Id)
}

func TestGetProof(t *testing.T) {
	mockServer, mockAddress := mockGetProofCall(t)
	defer mockServer.Close()

	restClient, _ := NewClient(mockAddress.Host)
	proofResponse, err := restClient.GetProof([]byte(billId))

	require.NoError(t, err)
	require.Len(t, proofResponse.Bills, 1)
	b, _ := base64.StdEncoding.DecodeString(billId)
	require.EqualValues(t, b, proofResponse.Bills[0].Id)
}

func TestBlockHeight(t *testing.T) {
	mockServer, mockAddress := mockGetBlockHeightCall(t)
	defer mockServer.Close()

	restClient, _ := NewClient(mockAddress.Host)
	blockHeight, err := restClient.GetBlockHeight()

	require.NoError(t, err)
	require.EqualValues(t, 1000, blockHeight)
}

func Test_NewClient(t *testing.T) {

	t.Run("invalid URL", func(t *testing.T) {
		mbc, err := NewClient("x:y:z")
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
			mbc, err := NewClient(tc.param)
			if err != nil {
				t.Errorf("unexpected error for parameter %q: %v", tc.param, err)
			}
			if mbc.baseUrl != tc.url {
				t.Errorf("expected URL for %q to be %q, got %q", tc.param, tc.url, mbc.baseUrl)
			}
		}
	})
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
		w.Write([]byte(`{"total": 8, "bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDcBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDcBill":false}]}`))
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
		if !r.URL.Query().Has("offset") {
			w.Write([]byte(`{"total": 13, "bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false}]}`))
		} else if r.URL.Query().Get("offset") == "5" {
			w.Write([]byte(`{"total": 13, "bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false}]}`))
		} else if r.URL.Query().Get("offset") == "10" {
			w.Write([]byte(`{"total": 13, "bills": [{"id":"` + billId + `","value":"10","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":"5","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false}]}`))
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetProofCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/"+ProofPath) {
			t.Errorf("Expected to request '%v', got: %s", ProofPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"bills":[{"id":"` + billId + `", "value":"10", "txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=", "isDcBill":false, "txProof":{"blockNumber":1, "tx":{"systemId":"AAAAAA==", "unitId":"Uv38ByGCZU8WP18PmmIdcpVmx00QA3xNe7sEB9Hixkk=", "transactionAttributes":{}, "timeout":10, "ownerProof":"gYVa"}, "proof":{"proofType":"PRIM", "blockHeaderHash":"AA==", "transactionsHash":"", "hashValue":"", "blockTreeHashChain":{"items":[{"val":"AA==", "hash":"AA=="}]}, "secTreeHashChain":null, "unicityCertificate":null}}}]}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetBlockHeightCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + BlockHeightPath) {
			t.Errorf("Expected to request '%v', got: %s", BlockHeightPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"blockHeight": "1000"}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}
