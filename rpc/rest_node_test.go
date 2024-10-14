package rpc

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/stretchr/testify/require"
)

func TestRESTServer_GetState_Ok(t *testing.T) {
	node := &MockNode{txs: &testtxsystem.CounterTxSystem{}}
	obs := observability.Default(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/state", bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, NodeEndpoints(node, obs)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
}

func TestRESTServer_GetState_Error(t *testing.T) {
	node := &MockNode{txs: &testtxsystem.CounterTxSystem{ErrorState: &testtxsystem.ErrorState{Err: errors.New("state error")}}}
	obs := observability.Default(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/state", bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, NodeEndpoints(node, obs)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusInternalServerError, recorder.Result().StatusCode)
	require.Contains(t, recorder.Body.String(), "state error")
}
