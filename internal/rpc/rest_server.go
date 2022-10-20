package rpc

import (
	"encoding/json"
	goerrors "errors"
	"net/http"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/gorilla/mux"
)

const (
	pathTransactions  = "/transactions"
	headerContentType = "Content-Type"
	applicationJson   = "application/json"
)

type RestServer struct {
	*http.Server
	node        partitionNode
	maxBodySize int64
}

func NewRESTServer(node partitionNode, addr string, maxBodySize int64) (*RestServer, error) {
	if node == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "partition node is nil")
	}
	rs := &RestServer{
		Server: &http.Server{
			Addr: addr,
		},
		maxBodySize: maxBodySize,
		node:        node,
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
	tx := &txsystem.Transaction{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(tx)
	if err != nil {
		writeError(writer, err, http.StatusBadRequest)
		return
	}
	err = s.node.SubmitTx(tx)
	if err != nil {
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
