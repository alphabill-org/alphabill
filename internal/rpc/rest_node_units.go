package rpc

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/tree/avl"
	"github.com/gorilla/mux"
)

func getUnit(node partitionNode, index keyvaluedb.KeyValueDB, log *slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		unitIDString := mux.Vars(r)["unitID"]
		unitID, err := hex.DecodeString(unitIDString)
		if err != nil {
			util.WriteCBORError(w, fmt.Errorf("invalid unit ID: %w", err), http.StatusBadRequest, log)
			return
		}

		query := r.URL.Query()
		fieldsString := strings.TrimSpace(query.Get("fields"))
		var returnUnitData bool
		var returnProof bool
		if fieldsString == "" {
			returnUnitData = true
			returnProof = true
		} else {
			fields := strings.Split(fieldsString, ",")
			for _, field := range fields {
				if field == "state" {
					returnUnitData = true
				} else if field == "state_proof" {
					returnProof = true
				} else {
					util.WriteCBORError(w, fmt.Errorf("invalid fields query parameter: %v", field), http.StatusBadRequest, log)
					return
				}

			}
		}
		txOrderHashString := strings.TrimSpace(query.Get("txOrderHash"))
		var txOrderHash []byte
		if txOrderHashString != "" {
			h, err := hex.DecodeString(txOrderHashString)
			if err != nil {
				util.WriteCBORError(w, fmt.Errorf("invalid tx order hash format: %w", err), http.StatusBadRequest, log)
				return
			}
			txOrderHash = h
		}
		if txOrderHash == nil {
			dataAndProof, err := node.GetUnitState(unitID, returnProof, returnUnitData)
			if err != nil {
				if errors.Is(err, avl.ErrNotFound) {
					util.WriteCBORError(w, errors.New("not found"), http.StatusNotFound, log)
				} else {
					util.WriteCBORError(w, err, http.StatusInternalServerError, log)
				}
				return
			}
			util.WriteCBORResponse(w, dataAndProof, http.StatusOK, log)
			return
		}
		response, err := partition.ReadUnitProofIndex(index, unitID, txOrderHash)
		if err != nil {
			if errors.Is(err, partition.ErrIndexNotFound) {
				util.WriteCBORError(w, errors.New("not found"), http.StatusNotFound, log)
			} else {
				util.WriteCBORError(w, err, http.StatusInternalServerError, log)
			}
			return
		}
		if !returnUnitData {
			response.UnitData = nil
		}
		if !returnProof {
			response.Proof = nil
		}
		util.WriteCBORResponse(w, response, http.StatusOK, log)
	}
}
