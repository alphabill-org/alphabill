package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"

	"github.com/stretchr/testify/require"
)

const MaxBodySize int64 = 1 << 20 // 1 MB

func TestNewRESTServer_PartitionNodeIsNil(t *testing.T) {
	s, err := NewRESTServer(nil, "", MaxBodySize)
	require.ErrorContains(t, err, "partition node is nil")
	require.Nil(t, s)
}

func TestNewRESTServer_OK(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.NotNil(t, s.Server)
	require.NotNil(t, s.node)
}

func TestNewRESTServer_SubmitTx(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize)
	require.NoError(t, err)

	transferTx, err := json.Marshal(testtransaction.RandomBillTransfer(t))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(transferTx))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusAccepted, recorder.Code)
}

func TestNewRESTServer_NotFound(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize)
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
	s, err := NewRESTServer(&MockNode{}, "", MaxBodySize)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(test.RandomBytes(102)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	s.Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid character")
}

func TestNewRESTServer_RequestBodyTooLarge(t *testing.T) {
	s, err := NewRESTServer(&MockNode{}, "", 10)
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
