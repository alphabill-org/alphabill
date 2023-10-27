package rpc

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/validator/pkg/testutils"
	"github.com/stretchr/testify/require"
)

const MaxBodySize int64 = 1 << 20 // 1 MB

func TestNewRESTServer_NotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/notfound", bytes.NewReader(test.RandomBytes(10)))
	recorder := httptest.NewRecorder()

	NewRESTServer("", MaxBodySize).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "404 page not found")
}
