package rpc

import (
	"bytes"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"net/http"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/gorilla/mux"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	pathTransactions  = "/transactions"
	headerContentType = "Content-Type"
	applicationJson   = "application/json"
)

type RestServer struct {
	*http.Server
	node                partitionNode
	supportedAttributes map[string]proto.Message
	maxBodySize         int64
}

type Request struct {
	SystemId              []byte         `json:"system_id,omitempty"`
	UnitId                []byte         `json:"unit_id,omitempty"`
	TransactionType       string         `json:"type,omitempty"`
	TransactionAttributes map[string]any `json:"attributes,omitempty"`
	Timeout               uint64         `json:"timeout,omitempty"`
	OwnerProof            []byte         `json:"owner_proof,omitempty"`
}

func NewRESTServer(node partitionNode, addr string, txTypes map[string]proto.Message, maxBodySize int64) (*RestServer, error) {
	if node == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "partition node is nil")
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
	}

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(notFound)
	handler := rs.submitTransaction
	if maxBodySize > 0 {
		handler = rs.maxBytesHandler(handler)
	}
	r.HandleFunc(pathTransactions, handler).Methods(http.MethodPost).Headers(headerContentType, applicationJson)
	rs.Handler = r
	return rs, nil
}

func (s *RestServer) submitTransaction(writer http.ResponseWriter, r *http.Request) {
	jsonReq := &Request{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(jsonReq)
	if err != nil {
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
		writeError(writer, err, http.StatusBadRequest)
		return
	}

	// TODO this code must be refactored after we have specified how hashing works in AB (AB-409).
	attrBytes, err := json.Marshal(jsonReq.TransactionAttributes)
	if err != nil {
		writeError(writer, err, http.StatusBadRequest)
		return
	}

	dec = json.NewDecoder(bytes.NewReader(attrBytes))
	dec.DisallowUnknownFields()
	err = dec.Decode(p)

	err = protoTx.TransactionAttributes.MarshalFrom(p)
	if err != nil {
		writeError(writer, err, http.StatusBadRequest)
		return
	}

	// submit to the node
	err = s.node.SubmitTx(protoTx)
	if err != nil {
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	writer.WriteHeader(http.StatusAccepted)
}

func (s *RestServer) getAttributesType(jsonReq *Request) (proto.Message, error) {
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
