package rpc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/gorilla/mux"
)

func NodeEndpoints(node partitionNode, obs Observability) RegistrarFunc {
	return func(r *mux.Router) {
		log := obs.Logger()

		// get the state file
		r.HandleFunc("/state", getState(node, log)).Methods("GET")
		r.HandleFunc("/configurations", putVar(node.RegisterValidatorAssignmentRecord)).Methods("PUT")
	}
}

func getState(node partitionNode, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := node.TransactionSystemState().Serialize(w, true); err != nil {
			w.Header().Set("Content-Type", "application/cbor")
			w.WriteHeader(http.StatusInternalServerError)
			if err := types.Cbor.Encode(w, struct {
				_   struct{} `cbor:",toarray"`
				Err string
			}{
				Err: fmt.Sprintf("%v", err),
			}); err != nil {
				log.Warn("failed to write CBOR error response", logger.Error(err))
			}
			return
		}
	}
}

func putVar(registerVAR func(v *partitions.ValidatorAssignmentRecord) error) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		v := &partitions.ValidatorAssignmentRecord{}
		if err := json.NewDecoder(request.Body).Decode(&v); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "parsing validator assignment record: %v", err)
			return
		}
		if err := registerVAR(v); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "registering validator assignment record: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
