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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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
		node                partitionNode
		supportedAttributes map[string]proto.Message
		maxBodySize         int64
		self                *network.Peer
	}

	SubmitTx struct {
		SystemId              []byte         `json:"system_id,omitempty"`
		UnitId                []byte         `json:"unit_id,omitempty"`
		TransactionType       string         `json:"type,omitempty"`
		TransactionAttributes map[string]any `json:"attributes,omitempty"`
		Timeout               uint64         `json:"timeout,omitempty"`
		OwnerProof            []byte         `json:"owner_proof,omitempty"`
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

func NewRESTServer(node partitionNode, addr string, txTypes map[string]proto.Message, maxBodySize int64, self *network.Peer) (*RestServer, error) {
	if node == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "partition node is nil")
	}
	if self == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "network peer is nil")
	}
	var types = map[string]proto.Message{}
	if txTypes != nil {
		types = txTypes
	}
	rs := &RestServer{
		Server: &http.Server{
			Addr: addr,
		},
		maxBodySize:         maxBodySize,
		node:                node,
		supportedAttributes: types,
		self:                self,
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
	jsonReq := &SubmitTx{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(jsonReq)
	if err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	protoTx := &txsystem.Transaction{
		SystemId:              jsonReq.SystemId,
		UnitId:                jsonReq.UnitId,
		Timeout:               jsonReq.Timeout,
		OwnerProof:            jsonReq.OwnerProof,
		TransactionAttributes: new(anypb.Any),
	}

	// get the type of the attributes struct
	p, err := s.getAttributesType(jsonReq)
	if err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}

	// TODO this code must be refactored after we have specified how hashing works in AB (AB-409).
	attrBytes, err := json.Marshal(jsonReq.TransactionAttributes)
	if err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}

	dec = json.NewDecoder(bytes.NewReader(attrBytes))
	dec.DisallowUnknownFields()
	err = dec.Decode(p)

	err = protoTx.TransactionAttributes.MarshalFrom(p)
	if err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}

	// submit to the node
	err = s.node.SubmitTx(protoTx)
	if err != nil {
		receivedInvalidTransactionsRESTMeter.Inc(1)
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	writer.WriteHeader(http.StatusAccepted)
}

func (s *RestServer) getAttributesType(jsonReq *SubmitTx) (proto.Message, error) {
	attrType := jsonReq.TransactionType
	attrProtoType, f := s.supportedAttributes[attrType]
	if !f {
		return nil, fmt.Errorf("unsupported type %v", attrType)
	}
	return proto.Clone(attrProtoType), nil
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
	w.WriteHeader(statusCode)
	w.Header().Set(headerContentType, applicationJson)
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
