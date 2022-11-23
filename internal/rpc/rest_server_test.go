package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/stretchr/testify/require"
)

const MaxBodySize int64 = 1 << 20 // 1 MB

func TestNewRESTServer_PartitionNodeIsNil(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(nil, "", money.TransactionTypes, MaxBodySize, peer)
	require.ErrorContains(t, err, "partition node is nil")
	require.Nil(t, s)
}
func TestNewRESTServer_PeerIsNil(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, MaxBodySize, nil)
	require.ErrorContains(t, err, "peer is nil")
	require.Nil(t, s)
}

func TestNewRESTServer_OK(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, MaxBodySize, peer)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.NotNil(t, s.Server)
	require.NotNil(t, s.node)
}

func TestMetrics_OK(t *testing.T) {
	node := &MockNode{}
	metrics.Enabled = true
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(node, "", money.TransactionTypes, MaxBodySize, peer)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	recorder := httptest.NewRecorder()
	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "transactions_grpc_received")
}

func TestNewRESTServer_SubmitTx(t *testing.T) {
	node := &MockNode{}
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(node, "", money.TransactionTypes, MaxBodySize, peer)
	require.NoError(t, err)
	jsonRequest := &SubmitTx{
		SystemId:        []byte{0, 0, 0, 0},
		UnitId:          []byte{0, 0, 0, 1},
		TransactionType: money.TypeTransferOrder,
		TransactionAttributes: map[string]any{
			"new_bearer":   []byte{0, 0, 1, 0},
			"target_value": uint64(1000),
			"backlink":     []byte{0, 0, 1, 1},
		},
		Timeout:    100,
		OwnerProof: []byte{0, 0, 0, 2},
	}
	transferTx, err := json.Marshal(jsonRequest)

	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(transferTx))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Equal(t, 1, len(node.transactions))
	tx := node.transactions[0]
	require.Equal(t, jsonRequest.SystemId, tx.SystemId)
	require.Equal(t, jsonRequest.UnitId, tx.UnitId)
	require.Equal(t, jsonRequest.OwnerProof, tx.OwnerProof)
	require.Equal(t, jsonRequest.Timeout, tx.Timeout)

	attr := &money.TransferOrder{}
	err = tx.TransactionAttributes.UnmarshalTo(attr)
	require.NoError(t, err)
	require.Equal(t, jsonRequest.TransactionAttributes["backlink"], attr.Backlink)
	require.Equal(t, jsonRequest.TransactionAttributes["new_bearer"], attr.NewBearer)
	require.Equal(t, jsonRequest.TransactionAttributes["target_value"], attr.TargetValue)
}

func TestNewRESTServer_NotFound(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, MaxBodySize, peer)
	require.NoError(t, err)

	transferTx, err := json.Marshal(moneytesttx.RandomBillTransfer(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/notfound", bytes.NewReader(transferTx))
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "404 not found")
}

func TestNewRESTServer_InvalidTx(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, MaxBodySize, peer)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(test.RandomBytes(102)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "json decode error")
}

func TestNewRESTServer_RequestBodyTooLarge(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, 10, peer)
	require.NoError(t, err)
	transferTx, err := json.Marshal(moneytesttx.RandomBillTransfer(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(transferTx))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "http: request body too large")
}

func TestRESTServer_RequestInfo(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, 10, peer)
	require.NoError(t, err)
	transferTx, err := json.Marshal(moneytesttx.RandomBillTransfer(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", bytes.NewReader(transferTx))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	response := &infoResponse{}
	err = json.NewDecoder(recorder.Body).Decode(response)
	require.NoError(t, err)
	require.Equal(t, "00010000", response.SystemID)
	require.Equal(t, peer.ID().String(), response.Self.Identifier)
	require.Equal(t, 1, len(response.Self.Addresses))
	require.Equal(t, peer.MultiAddresses(), response.Self.Addresses)
	require.Equal(t, 0, len(response.OpenConnections))
	require.Equal(t, 0, len(response.PartitionValidators))
	require.Equal(t, 0, len(response.RootValidators))
}
