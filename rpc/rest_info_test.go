package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/stretchr/testify/require"
)

func TestRESTServer_RequestInfo(t *testing.T) {
	peerConf := peer.CreatePeerConfiguration(t)
	peer := peer.CreatePeer(t, peerConf)

	observe := observability.NOPObservability()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	NewRESTServer("", 10, observe, InfoEndpoints(&MockNode{}, "mock node", peer, observe.Logger())).Handler.ServeHTTP(recorder, req)
	response := &infoResponse{}
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(response))
	require.Equal(t, "00010000", response.SystemID)
	require.Equal(t, "mock node", response.Name)
	require.Equal(t, peer.ID().String(), response.Self.Identifier)
	require.Equal(t, 1, len(response.Self.Addresses))
	require.Equal(t, peer.MultiAddresses(), response.Self.Addresses)
	require.Equal(t, 0, len(response.OpenConnections))
	require.Equal(t, 0, len(response.PartitionValidators))
	require.Equal(t, 0, len(response.RootValidators))
}
