package rpc

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/gorilla/mux"
)

func getUnit(node partitionNode, index keyvaluedb.KeyValueDB, log *slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		unitIDString := mux.Vars(r)["unitID"]
		unitID, err := hex.DecodeString(unitIDString)
		if err != nil {
			WriteCBORError(w, fmt.Errorf("invalid unit ID: %w", err), http.StatusBadRequest, log)
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
					WriteCBORError(w, fmt.Errorf("invalid fields query parameter: %v", field), http.StatusBadRequest, log)
					return
				}

			}
		}
		txOrderHashString := strings.TrimSpace(query.Get("txOrderHash"))
		var txOrderHash []byte
		if txOrderHashString != "" {
			h, err := hex.DecodeString(txOrderHashString)
			if err != nil {
				WriteCBORError(w, fmt.Errorf("invalid tx order hash format: %w", err), http.StatusBadRequest, log)
				return
			}
			txOrderHash = h
		}
		if txOrderHash == nil {
			dataAndProof, err := node.GetUnitState(unitID, returnProof, returnUnitData)
			if err != nil {
				if errors.Is(err, avl.ErrNotFound) {
					WriteCBORError(w, errors.New("not found"), http.StatusNotFound, log)
				} else {
					WriteCBORError(w, err, http.StatusInternalServerError, log)
				}
				return
			}
			WriteCBORResponse(w, dataAndProof, http.StatusOK, log)
			return
		}
		response, err := partition.ReadUnitProofIndex(index, unitID, txOrderHash)
		if err != nil {
			if errors.Is(err, partition.ErrIndexNotFound) {
				WriteCBORError(w, errors.New("not found"), http.StatusNotFound, log)
			} else {
				WriteCBORError(w, err, http.StatusInternalServerError, log)
			}
			return
		}
		if !returnUnitData {
			response.UnitData = nil
		}
		if !returnProof {
			response.Proof = nil
		}
		WriteCBORResponse(w, response, http.StatusOK, log)
	}
}
