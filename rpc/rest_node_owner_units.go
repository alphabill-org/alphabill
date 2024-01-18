package rpc

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
)

func getOwnerUnits(node partitionNode, log *slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerIDString := mux.Vars(r)["ownerID"]
		ownerID, err := hex.DecodeString(ownerIDString)
		if err != nil {
			WriteCBORError(w, fmt.Errorf("invalid owner ID: %w", err), http.StatusBadRequest, log)
			return
		}
		unitIDs, err := node.GetOwnerUnits(ownerID)
		if err != nil {
			WriteCBORError(w, err, http.StatusInternalServerError, log)
			return
		}
		WriteCBORResponse(w, unitIDs, http.StatusOK, log)
	}
}
