package rpc

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/gorilla/mux"
)

func NodeEndpoints(node partitionNode, obs Observability) RegistrarFunc {
	return func(r *mux.Router) {
		log := obs.Logger()

		// get the state file
		r.HandleFunc("/state", getState(node, log))
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
