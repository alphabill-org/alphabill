package rpc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/common/util"
	"github.com/alphabill-org/alphabill/validator/pkg/metrics"
	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/mux"
)

const (
	pathTransactions         = "/transactions"
	pathGetTransactionRecord = "/transactions/{txOrderHash}"
	pathLatestRoundNumber    = "/rounds/latest"
)

var (
	receivedTransactionsRESTMeter        = metrics.GetOrRegisterCounter("transactions/rest/received")
	receivedInvalidTransactionsRESTMeter = metrics.GetOrRegisterCounter("transactions/rest/invalid")
)

func NodeEndpoints(node partitionNode, log *slog.Logger) RegistrarFunc {
	return func(r *mux.Router) {

		// submit transaction
		r.HandleFunc(pathTransactions, submitTransaction(node, log)).Methods(http.MethodPost, http.MethodOptions)

		// get transaction record and execution proof
		r.HandleFunc(pathGetTransactionRecord, getTransactionRecord(node, log)).Methods(http.MethodGet, http.MethodOptions)

		// get latest round number
		r.HandleFunc(pathLatestRoundNumber, getLatestRoundNumber(node, log))
	}
}

func submitTransaction(node partitionNode, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		receivedTransactionsRESTMeter.Inc(1)
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(r.Body); err != nil {
			receivedInvalidTransactionsRESTMeter.Inc(1)
			util.WriteCBORError(w, fmt.Errorf("reading request body failed: %w", err), http.StatusBadRequest, log)
			return
		}

		tx := &types.TransactionOrder{}
		if err := cbor.Unmarshal(buf.Bytes(), tx); err != nil {
			receivedInvalidTransactionsRESTMeter.Inc(1)
			util.WriteCBORError(w, fmt.Errorf("unable to decode request body as transaction: %w", err), http.StatusBadRequest, log)
			return
		}
		txOrderHash, err := node.SubmitTx(r.Context(), tx)
		if err != nil {
			receivedInvalidTransactionsRESTMeter.Inc(1)
			util.WriteCBORError(w, err, http.StatusBadRequest, log)
			return
		}
		util.WriteCBORResponse(w, txOrderHash, http.StatusAccepted, log)
	}
}

func getTransactionRecord(node partitionNode, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		txOrder := vars["txOrderHash"]
		txOrderHash, err := hex.DecodeString(txOrder)
		if err != nil {
			util.WriteCBORError(w, fmt.Errorf("invalid tx order hash: %s", txOrder), http.StatusBadRequest, log)
			return
		}
		txRecord, proof, err := node.GetTransactionRecord(r.Context(), txOrderHash)
		if err != nil {
			util.WriteCBORError(w, err, http.StatusInternalServerError, log)
			return
		}

		if txRecord == nil {
			util.WriteCBORError(w, errors.New("not found"), http.StatusNotFound, log)
			return
		}

		util.WriteCBORResponse(w, struct {
			_        struct{} `cbor:",toarray"`
			TxRecord *types.TransactionRecord
			TxProof  *types.TxProof
		}{
			TxRecord: txRecord,
			TxProof:  proof,
		}, http.StatusOK, log)
	}
}

func getLatestRoundNumber(node partitionNode, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		nr, err := node.GetLatestRoundNumber()
		if err != nil {
			util.WriteCBORError(w, err, http.StatusInternalServerError, log)
			return
		}
		util.WriteCBORResponse(w, nr, http.StatusOK, log)
	}
}
