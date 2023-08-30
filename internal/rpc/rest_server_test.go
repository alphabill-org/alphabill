package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

const MaxBodySize int64 = 1 << 20 // 1 MB

func TestNewRESTServer_PartitionNodeIsNil(t *testing.T) {
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(nil, "", MaxBodySize, p)
	require.ErrorContains(t, err, "can't initialize REST server with nil partition node")
	require.Nil(t, s)
}
func TestNewRESTServer_PeerIsNil(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize, nil)
	require.ErrorContains(t, err, "can't initialize REST server with nil network peer")
	require.Nil(t, s)
}

func TestNewRESTServer_OK(t *testing.T) {
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize, p)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.NotNil(t, s.Server)
	require.NotNil(t, s.node)
}

func TestMetrics_OK(t *testing.T) {
	node := &MockNode{}
	metrics.Enabled = true
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(node, "", MaxBodySize, p)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	recorder := httptest.NewRecorder()
	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "ab_transactions_grpc_received")
}

func TestRestServer_SubmitTransaction(t *testing.T) {
	node := &MockNode{}
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(node, "", MaxBodySize, p)
	require.NoError(t, err)
	transaction := createTxOrder(t)
	message, err := cbor.Marshal(transaction)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(message))
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Equal(t, 1, len(node.transactions))
	tx := node.transactions[0]
	require.Equal(t, transaction.SystemID(), tx.SystemID())
	require.Equal(t, transaction.UnitID(), tx.UnitID())
	require.Equal(t, transaction.OwnerProof, tx.OwnerProof)
	require.Equal(t, transaction.Timeout(), tx.Timeout())
	require.Equal(t, transaction.PayloadType(), tx.PayloadType())
	require.Equal(t, transaction.Payload.Attributes, tx.Payload.Attributes)
}

func createTxOrder(t *testing.T) *types.TransactionOrder {
	transaction := testtransaction.NewTransactionOrder(t,
		testtransaction.WithAttributes([]byte{0, 0, 0, 0, 0, 0, 0}),
		testtransaction.WithUnitId([]byte{0, 0, 0, 1}),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithOwnerProof([]byte{0, 0, 0, 2}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: 100}),
		testtransaction.WithPayloadType("test"),
	)
	return transaction
}

func TestRestServer_TransactionOptions(t *testing.T) {
	node := &MockNode{}
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(node, "", MaxBodySize, p)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/transactions", nil)
	recorder := httptest.NewRecorder()
	s.Handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "POST,OPTIONS", recorder.Header().Get("Access-Control-Allow-Methods"))
	require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, headerContentType, recorder.Header().Get("Access-Control-Allow-Headers"))
	require.Empty(t, recorder.Body)
}

func TestNewRESTServer_NotFound(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize, peer)
	require.NoError(t, err)

	transferTx, err := cbor.Marshal(createTxOrder(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/notfound", bytes.NewReader(transferTx))
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "request path doesn't match any endpoint")
}

func TestNewRESTServer_InvalidTx(t *testing.T) {
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize, p)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(test.RandomBytes(102)))
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "failed to decode request body as transaction")
}

func TestNewRESTServer_RequestBodyTooLarge(t *testing.T) {
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", 10, p)
	require.NoError(t, err)
	transferTx, err := cbor.Marshal(createTxOrder(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(transferTx))
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "http: request body too large")
}

func TestRESTServer_RequestInfo(t *testing.T) {
	p := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", 10, p)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	response := &infoResponse{}
	err = json.NewDecoder(recorder.Body).Decode(response)
	require.NoError(t, err)
	require.Equal(t, "00010000", response.SystemID)
	require.Equal(t, p.ID().String(), response.Self.Identifier)
	require.Equal(t, 1, len(response.Self.Addresses))
	require.Equal(t, p.MultiAddresses(), response.Self.Addresses)
	require.Equal(t, 0, len(response.OpenConnections))
	require.Equal(t, 0, len(response.PartitionValidators))
	require.Equal(t, 0, len(response.RootValidators))
}

func TestRESTServer_GetLatestRoundNumber(t *testing.T) {
	node := &MockNode{}
	s, err := NewRESTServer(node, "", 10, peer.CreatePeer(t))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rounds/latest", bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
	var response uint64
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(&response))
	require.Equal(t, node.maxBlockNumber, response)
}

func TestRESTServer_GetTransactionRecord_OK(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", 10, peer.CreatePeer(t))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/transactions/%s", hex.EncodeToString(test.RandomBytes(32))), bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	s.Handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
	require.Equal(t, applicationCBOR, recorder.Result().Header.Get(headerContentType))

	response := struct {
		_        struct{} `cbor:",toarray"`
		TxRecord *types.TransactionRecord
		TxProof  *types.TxProof
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(&response))
	require.NotNil(t, response.TxRecord)
	require.NotNil(t, response.TxProof)
}

func TestRESTServer_GetTransactionRecord_NotFound(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", 10, peer.CreatePeer(t))
	require.NoError(t, err)

	var hash [32]byte

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/transactions/%s", hex.EncodeToString(hash[:])), bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	s.Handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Result().StatusCode)
	require.Equal(t, int64(-1), recorder.Result().ContentLength)
}
