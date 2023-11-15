package rpc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
)

func TestMetrics_OK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	recorder := httptest.NewRecorder()
	var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ab_transactions_grpc_received")
	}
	NewRESTServer("", MaxBodySize, observability.NOPMetrics(), logger.NOP(), MetricsEndpoints(h)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "ab_transactions_grpc_received")
}
