package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/stretchr/testify/require"
)

func TestRESTServer_RequestInfo(t *testing.T) {
	p := peer.CreatePeer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	NewRESTServer("", 10, InfoEndpoints(&MockNode{}, p)).Handler.ServeHTTP(recorder, req)
	response := &infoResponse{}
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(response))
	require.Equal(t, "00010000", response.SystemID)
	require.Equal(t, p.ID().String(), response.Self.Identifier)
	require.Equal(t, 1, len(response.Self.Addresses))
	require.Equal(t, p.MultiAddresses(), response.Self.Addresses)
	require.Equal(t, 0, len(response.OpenConnections))
	require.Equal(t, 0, len(response.PartitionValidators))
	require.Equal(t, 0, len(response.RootValidators))
}
