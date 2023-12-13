package rpc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/txbuffer"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	pathTransactions         = "/transactions"
	pathGetTransactionRecord = "/transactions/{txOrderHash}"
	pathLatestRoundNumber    = "/rounds/latest"
	pathState                = "/state"
	pathUnits                = "/units/{unitID}"
)

func NodeEndpoints(node partitionNode, unitProofDB keyvaluedb.KeyValueDB, obs Observability, log *slog.Logger) RegistrarFunc {
	return func(r *mux.Router) {
		// submit transaction
		r.HandleFunc(pathTransactions, submitTransaction(node, obs.Meter(metricsScopeRESTAPI), log)).Methods(http.MethodPost, http.MethodOptions)

		// get transaction record and execution proof
		r.HandleFunc(pathGetTransactionRecord, getTransactionRecord(node, log)).Methods(http.MethodGet, http.MethodOptions)

		// get latest round number
		r.HandleFunc(pathLatestRoundNumber, getLatestRoundNumber(node, log)).Methods(http.MethodGet, http.MethodOptions)

		// get unit data and proof
		r.HandleFunc(pathUnits, getUnit(node, unitProofDB, log)).Methods(http.MethodGet, http.MethodOptions)
		// Queries("txOrderHash", "{txOrderHash}", "fields", "{fields}") - fields are not mandatory

		// get the state file
		r.HandleFunc(pathState, getState(node, log))
	}
}

func submitTransaction(node partitionNode, mtr metric.Meter, log *slog.Logger) http.HandlerFunc {
	const txStatusKey = attribute.Key("status")

	txCnt, err := mtr.Int64Counter("tx.count", metric.WithUnit("{transaction}"), metric.WithDescription("Number of transactions submitted"))
	if err != nil {
		log.Error("creating tx.count metric", logger.Error(err))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(r.Body); err != nil {
			txCnt.Add(r.Context(), 1, metric.WithAttributes(txStatusKey.String("err.read")))
			WriteCBORError(w, fmt.Errorf("reading request body failed: %w", err), http.StatusBadRequest, log)
			return
		}

		tx := &types.TransactionOrder{}
		if err := cbor.Unmarshal(buf.Bytes(), tx); err != nil {
			txCnt.Add(r.Context(), 1, metric.WithAttributes(txStatusKey.String("err.cbor")))
			WriteCBORError(w, fmt.Errorf("unable to decode request body as transaction: %w", err), http.StatusBadRequest, log)
			return
		}
		txOrderHash, err := node.SubmitTx(r.Context(), tx)
		txCnt.Add(r.Context(), 1, metric.WithAttributes(attribute.Key("tx").String(tx.PayloadType()), txStatusKey.String(statusCodeOfTxError(err))))
		if err != nil {
			WriteCBORError(w, err, http.StatusBadRequest, log)
			return
		}
		WriteCBORResponse(w, txOrderHash, http.StatusAccepted, log)
	}
}

func statusCodeOfTxError(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, txbuffer.ErrTxBufferFull):
		return "buf.full"
	case errors.Is(err, txbuffer.ErrTxInBuffer):
		return "buf.double"
	case errors.Is(err, txbuffer.ErrTxIsNil):
		return "nil"
	case errors.Is(err, partition.ErrTxTimeout):
		return "tx.timeout"
	default:
		return "err"
	}
}

func getTransactionRecord(node partitionNode, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		txOrder := vars["txOrderHash"]
		txOrderHash, err := hex.DecodeString(txOrder)
		if err != nil {
			WriteCBORError(w, fmt.Errorf("invalid tx order hash: %s", txOrder), http.StatusBadRequest, log)
			return
		}
		txRecord, proof, err := node.GetTransactionRecord(r.Context(), txOrderHash)
		if err != nil {
			if errors.Is(err, partition.ErrIndexNotFound) {
				WriteCBORError(w, errors.New("not found"), http.StatusNotFound, log)
			}
			WriteCBORError(w, err, http.StatusInternalServerError, log)
			return
		}

		WriteCBORResponse(w, struct {
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
		nr, err := node.GetLatestRoundNumber(request.Context())
		if err != nil {
			WriteCBORError(w, err, http.StatusInternalServerError, log)
			return
		}
		WriteCBORResponse(w, nr, http.StatusOK, log)
	}
}

func getState(node partitionNode, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/cbor")
		w.WriteHeader(http.StatusOK)

		if err := node.WriteStateFile(w); err != nil {
			log.Error("writing state file", logger.Error(err))
		}
	}
}
