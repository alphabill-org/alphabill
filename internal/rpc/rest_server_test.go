package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const MaxBodySize int64 = 1 << 20 // 1 MB

func TestNewRESTServer_PartitionNodeIsNil(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(nil, "", MaxBodySize, peer)
	require.ErrorContains(t, err, "partition node is nil")
	require.Nil(t, s)
}
func TestNewRESTServer_PeerIsNil(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize, nil)
	require.ErrorContains(t, err, "peer is nil")
	require.Nil(t, s)
}

func TestNewRESTServer_OK(t *testing.T) {
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize, peer)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.NotNil(t, s.Server)
	require.NotNil(t, s.node)
}

func TestMetrics_OK(t *testing.T) {
	node := &MockNode{}
	metrics.Enabled = true
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(node, "", MaxBodySize, peer)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	recorder := httptest.NewRecorder()
	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "transactions_grpc_received")
}

func TestRestServer_SubmitTransaction(t *testing.T) {
	tests := []struct {
		name            string
		givenAttributes proto.Message
		expectedType    proto.Message
	}{
		{
			name:            "transfer",
			givenAttributes: moneytesttx.RandomTransferAttributes(),
			expectedType:    &money.TransferAttributes{},
		},
		{
			name:            "split",
			givenAttributes: moneytesttx.RandomSplitAttributes(),
			expectedType:    &money.SplitAttributes{},
		},
		{
			name:            "transferDC",
			givenAttributes: moneytesttx.RandomTransferDCAttributes(),
			expectedType:    &money.TransferDCAttributes{},
		},
		{
			name:            "swap",
			givenAttributes: moneytesttx.CreateRandomSwapDCAttributes(t, 2),
			expectedType:    &money.SwapDCAttributes{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &MockNode{}
			peer := peer.CreatePeer(t)
			s, err := NewRESTServer(node, "", MaxBodySize, peer)
			require.NoError(t, err)
			transaction := testtransaction.NewTransaction(t,
				testtransaction.WithAttributes(tt.givenAttributes),
				testtransaction.WithUnitId([]byte{0, 0, 0, 1}),
				testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
				testtransaction.WithOwnerProof([]byte{0, 0, 0, 2}),
				testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: 100}),
			)
			message, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(transaction)
			require.NoError(t, err)

			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(message))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			s.Handler.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusAccepted, recorder.Code)
			require.Equal(t, 1, len(node.transactions))
			tx := node.transactions[0]
			require.Equal(t, transaction.SystemId, tx.SystemId)
			require.Equal(t, transaction.UnitId, tx.UnitId)
			require.Equal(t, transaction.OwnerProof, tx.OwnerProof)
			require.Equal(t, transaction.Timeout(), tx.Timeout())

			err = tx.TransactionAttributes.UnmarshalTo(tt.expectedType)
			require.NoError(t, err)
			require.True(t, proto.Equal(tt.givenAttributes, tt.expectedType))
		})
	}
}

func TestRestServer_TransactionOptions(t *testing.T) {
	node := &MockNode{}
	peer := peer.CreatePeer(t)
	s, err := NewRESTServer(node, "", MaxBodySize, peer)
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
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize, peer)
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
	s, err := NewRESTServer(&MockNode{}, "", 10, peer)
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
	s, err := NewRESTServer(&MockNode{}, "", 10, peer)
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
