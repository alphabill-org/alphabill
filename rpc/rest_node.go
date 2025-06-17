package rpc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/gorilla/mux"
)

func NodeEndpoints(node partitionNode, obs Observability) RegistrarFunc {
	return func(r *mux.Router) {
		log := obs.Logger()

		// get the state file
		r.HandleFunc("/state", getState(node, log)).Methods("GET")
		r.HandleFunc("/configurations", putShardConf(node.RegisterShardConf)).Methods("PUT")
	}
}

func getState(node partitionNode, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := node.SerializeState(w); err != nil {
			w.Header().Set("Content-Type", "application/cbor")
			w.WriteHeader(http.StatusInternalServerError)
			if err := cbor.Encode(w, struct {
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

func putShardConf(registerShardConf func(shardConf *types.PartitionDescriptionRecord) error) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		shardConf := &types.PartitionDescriptionRecord{}
		if err := json.NewDecoder(request.Body).Decode(&shardConf); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "failed to parse shard conf: %v", err)
			return
		}
		if err := registerShardConf(shardConf); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "failed to register shard conf: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
