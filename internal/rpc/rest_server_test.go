package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/stretchr/testify/require"
)

const MaxBodySize int64 = 1 << 20 // 1 MB

func TestNewRESTServer_PartitionNodeIsNil(t *testing.T) {
	s, err := NewRESTServer(nil, "", money.TransactionTypes, MaxBodySize)
	require.ErrorContains(t, err, "partition node is nil")
	require.Nil(t, s)
}

func TestNewRESTServer_OK(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, MaxBodySize)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.NotNil(t, s.Server)
	require.NotNil(t, s.node)
}

func TestNewRESTServer_SubmitTx(t *testing.T) {
	node := &MockNode{}
	s, err := NewRESTServer(node, "", money.TransactionTypes, MaxBodySize)
	require.NoError(t, err)
	jsonRequest := &Request{
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
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(transferTx))
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
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, MaxBodySize)
	require.NoError(t, err)

	transferTx, err := json.Marshal(testtransaction.RandomBillTransfer(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/notfound", bytes.NewReader(transferTx))
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "404 not found")
}

func TestNewRESTServer_InvalidTx(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, MaxBodySize)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(test.RandomBytes(102)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid character")
}

func TestNewRESTServer_RequestBodyTooLarge(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", money.TransactionTypes, 10)
	require.NoError(t, err)
	transferTx, err := json.Marshal(testtransaction.RandomBillTransfer(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(transferTx))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "http: request body too large")
}
