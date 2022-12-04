package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"net/http"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/gorilla/mux"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/encoding/protojson"
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
		node        partitionNode
		maxBodySize int64
		self        *network.Peer
	}

	infoResponse struct {
		SystemID            string     `json:"system_id"` // hex encoded system identifier
		Self                peerInfo   `json:"self"`      // information about this peer
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
		return nil, errors.Wrap(errors.ErrInvalidArgument, "partition node is nil")
	}
	if self == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "network peer is nil")
	}
	rs := &RestServer{
		Server: &http.Server{
			Addr: addr,
		},
		maxBodySize: maxBodySize,
		node:        node,
		self:        self,
	}

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(notFound)
	handler := rs.submitTransaction
	if maxBodySize > 0 {
		handler = rs.maxBytesHandler(handler)
	}
	if metrics.Enabled() {
		r.Handle("/api/v1/metrics", metrics.PrometheusHandler()).Methods(http.MethodGet)
	}

	r.HandleFunc("/api/v1/info", rs.infoHandler).Methods(http.MethodGet)
	r.HandleFunc(pathTransactions, handler).Methods(http.MethodPost).Headers(headerContentType, applicationJson)
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
	tx := &txsystem.Transaction{}
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	if err != nil {
		err = fmt.Errorf("reading request body failed: %w", err)
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	bb := buf.Bytes()
	err = protojson.Unmarshal(bb, tx)
	if err != nil {
		err = fmt.Errorf("json decode error: %w", err)
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	err = s.node.SubmitTx(tx)
	if err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	writer.WriteHeader(http.StatusAccepted)
}

func (s *RestServer) maxBytesHandler(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBodySize)
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

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, goerrors.New("404 not found"), http.StatusNotFound)
}

func writeError(w http.ResponseWriter, e error, statusCode int) {
	w.Header().Set(headerContentType, applicationJson)
	w.WriteHeader(statusCode)
	var errStr = e.Error()
	aberror, ok := e.(*errors.AlphabillError)
	if ok {
		errStr = aberror.Message()
	}
	err := json.NewEncoder(w).Encode(
		struct {
			Error string `json:"error"`
		}{errStr})
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
