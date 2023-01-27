package client

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const pubKeyHex = "0x038003e218eea360cbf580ebb90cc8c8caf0ccef4bf660ea9ab4fc06b5c367b038"
const billId = "MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg="

func TestGetBalance(t *testing.T) {
	mockServer, mockAddress := mockGetBalanceCall(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	restClient := NewClient(mockAddress.Host)
	balance, err := restClient.getBalance(pubKey, true)

	require.NoError(t, err)
	require.EqualValues(t, 15, balance)
}

func TestListBills(t *testing.T) {
	mockServer, mockAddress := mockListBillsCall(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	restClient := NewClient(mockAddress.Host)
	billsResponse, err := restClient.listBills(pubKey)

	require.NoError(t, err)
	require.Len(t, billsResponse.Bills, 2)
	require.EqualValues(t, 15, billsResponse.Total)
	require.EqualValues(t, billId, billsResponse.Bills[0].Id)
}

func TestGetProof(t *testing.T) {
	mockServer, mockAddress := mockGetProofCall(t)
	defer mockServer.Close()

	pubKey, err := hexutil.Decode(pubKeyHex)
	restClient := NewClient(mockAddress.Host)
	proofResponse, err := restClient.getProof(pubKey, []byte(billId))

	require.NoError(t, err)
	require.Len(t, proofResponse.Proofs, 1)
	require.EqualValues(t, billId, proofResponse.Proofs[0].Id)
}

func TestBlockHeight(t *testing.T) {
	mockServer, mockAddress := mockGetBlockHeightCall(t)
	defer mockServer.Close()

	restClient := NewClient(mockAddress.Host)
	blockHeight, err := restClient.getBlockHeight()

	require.NoError(t, err)
	require.EqualValues(t, 1000, blockHeight)
}

func mockGetBalanceCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + balancePath) {
			t.Errorf("Expected to request '%v', got: %s", balancePath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"balance": 15}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockListBillsCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + listBillsPath) {
			t.Errorf("Expected to request '%v', got: %s", listBillsPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total": 15, "bills": [{"id":"` + billId + `","value":10,"txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},{"id":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","value":5,"txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false}]}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetProofCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/"+proofPath) {
			t.Errorf("Expected to request '%v', got: %s", proofPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"bills":[{"id":"` + billId + `", "value":10, "txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=", "isDcBill":false, "txProof":{"blockNumber":1, "tx":{"systemId":"AAAAAA==", "unitId":"Uv38ByGCZU8WP18PmmIdcpVmx00QA3xNe7sEB9Hixkk=", "transactionAttributes":{}, "timeout":10, "ownerProof":"gYVa"}, "proof":{"proofType":"PRIM", "blockHeaderHash":"AA==", "transactionsHash":"", "hashValue":"", "blockTreeHashChain":{"items":[{"val":"AA==", "hash":"AA=="}]}, "secTreeHashChain":null, "unicityCertificate":null}}}]}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func mockGetBlockHeightCall(t *testing.T) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ("/" + blockHeightPath) {
			t.Errorf("Expected to request '%v', got: %s", blockHeightPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"blockHeight": 1000}`))
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}
