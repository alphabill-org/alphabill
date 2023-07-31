package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/types"

	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/gorilla/mux"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/exp/slices"
)

const (
	pathTransactions  = "/api/v1/transactions"
	headerContentType = "Content-Type"
	applicationJson   = "application/json"
)

var receivedTransactionsRESTMeter = metrics.GetOrRegisterCounter("transactions/rest/received")
var receivedInvalidTransactionsRESTMeter = metrics.GetOrRegisterCounter("transactions/rest/invalid")

type (
	RestServer struct {
		*http.Server
		node partitionNode
		self *network.Peer
	}

	infoResponse struct {
		SystemID            string     `json:"system_id"` // hex encoded system identifier
		Self                peerInfo   `json:"self"`      // information about this peer
		BootstrapNodes      []peerInfo `json:"bootstrap_nodes"`
		RootValidators      []peerInfo `json:"root_validators"`
		PartitionValidators []peerInfo `json:"partition_validators"`
		OpenConnections     []peerInfo `json:"open_connections"` // all libp2p connections to other peers in the network

	}

	peerInfo struct {
		Identifier string                `json:"identifier"`
		Addresses  []multiaddr.Multiaddr `json:"addresses"`
	}
)

func NewRESTServer(node partitionNode, addr string, maxBodySize int64, self *network.Peer) (*RestServer, error) {
	if node == nil {
		return nil, errors.New("can't initialize REST server with nil partition node")
	}
	if self == nil {
		return nil, errors.New("can't initialize REST server with nil network peer")
	}

	rs := &RestServer{
		Server: &http.Server{
			Addr:              addr,
			ReadTimeout:       3 * time.Second,
			ReadHeaderTimeout: time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       30 * time.Second,
		},
		node: node,
		self: self,
	}

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(notFound)

	handler := rs.submitTransaction
	if maxBodySize > 0 {
		handler = maxBytesHandler(handler, maxBodySize)
	}
	r.HandleFunc(pathTransactions, handler).Methods(http.MethodPost)
	r.HandleFunc(pathTransactions, func(w http.ResponseWriter, _ *http.Request) {
		setCorsHeaders(w)
	}).Methods(http.MethodOptions)

	if metrics.Enabled() {
		r.Handle("/api/v1/metrics", metrics.PrometheusHandler()).Methods(http.MethodGet)
	}
	r.HandleFunc("/api/v1/info", rs.infoHandler).Methods(http.MethodGet)

	r.Use(mux.CORSMethodMiddleware(r))
	rs.Handler = r
	return rs, nil
}

func (s *RestServer) infoHandler(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("Handling '/api/v1/info' request")
	i := infoResponse{
		SystemID: hex.EncodeToString(s.node.SystemIdentifier()),
		Self: peerInfo{
			Identifier: s.self.ID().String(),
			Addresses:  s.self.MultiAddresses(),
		},
		BootstrapNodes:      s.getBootstrapNodes(),
		RootValidators:      s.getRootValidators(),
		PartitionValidators: s.getPartitionValidators(),
		OpenConnections:     s.getOpenConnections(),
	}
	w.Header().Set(headerContentType, applicationJson)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(i)
	if err != nil {
		logger.Warning("Failed to write info message: %v", err)
	}
}

func (s *RestServer) submitTransaction(writer http.ResponseWriter, r *http.Request) {
	receivedTransactionsRESTMeter.Inc(1)
	setCorsHeaders(writer)

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r.Body); err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, fmt.Errorf("reading request body failed: %w", err), http.StatusBadRequest)
		return
	}

	tx := &types.TransactionOrder{}
	if err := cbor.Unmarshal(buf.Bytes(), tx); err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, fmt.Errorf("failed to decode request body as transaction: %w", err), http.StatusBadRequest)
		return
	}

	if err := s.node.SubmitTx(r.Context(), tx); err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	writer.WriteHeader(http.StatusAccepted)
}

func maxBytesHandler(f http.HandlerFunc, maxBodySize int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		f(w, r)
	}
}

func (s *RestServer) getPartitionValidators() []peerInfo {
	validators := s.self.Validators()
	peers := make([]peerInfo, len(validators))
	peerStore := s.self.Network().Peerstore()
	for i, v := range validators {
		peers[i] = peerInfo{
			Identifier: v.String(),
			Addresses:  peerStore.PeerInfo(v).Addrs,
		}
	}
	return peers
}

func (s *RestServer) getOpenConnections() []peerInfo {
	connections := s.self.Network().Conns()
	peers := make([]peerInfo, len(connections))
	for i, connection := range connections {
		peers[i] = peerInfo{
			Identifier: connection.RemotePeer().String(),
			Addresses:  []multiaddr.Multiaddr{connection.RemoteMultiaddr()},
		}
	}
	return peers
}

func (s *RestServer) getRootValidators() []peerInfo {
	var peers []peerInfo
	peerStore := s.self.Network().Peerstore()
	ids := peerStore.Peers()
	for _, id := range ids {
		protocols, err := peerStore.SupportsProtocols(id, network.ProtocolBlockCertification)
		if err != nil {
			logger.Warning("Failed to query peer store: %v", err)
			continue
		}
		if slices.Contains(protocols, network.ProtocolBlockCertification) {
			peers = append(peers, peerInfo{
				Identifier: id.String(),
				Addresses:  peerStore.PeerInfo(id).Addrs,
			})
		}
	}
	return peers
}

func (s *RestServer) getBootstrapNodes() []peerInfo {
	bootstrapPeers := s.self.Configuration().BootstrapPeers
	infos := make([]peerInfo, len(bootstrapPeers))
	for i, p := range bootstrapPeers {
		infos[i] = peerInfo{Identifier: p.ID.String(), Addresses: p.Addrs}
	}
	return infos
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, errors.New("request path doesn't match any endpoint"), http.StatusNotFound)
}

func writeError(w http.ResponseWriter, e error, statusCode int) {
	w.Header().Set(headerContentType, applicationJson)
	w.WriteHeader(statusCode)

	errMsg := e.Error()
	if abErr, ok := e.(*aberrors.AlphabillError); ok {
		errMsg = abErr.Message()
	}
	err := json.NewEncoder(w).Encode(
		struct {
			Error string `json:"error"`
		}{errMsg})
	if err != nil {
		logger.Warning("Failed to encode error message: %v", err)
	}
}

func (pi *peerInfo) UnmarshalJSON(data []byte) error {
	var d map[string]interface{}
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	pi.Identifier, _ = d["identifier"].(string)
	addrs := d["addresses"].([]interface{})
	for _, addr := range addrs {
		multiAddr, err := multiaddr.NewMultiaddr(addr.(string))
		if err != nil {
			return err
		}
		pi.Addresses = append(pi.Addresses, multiAddr)
	}
	return nil
}

func setCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", headerContentType)
}
