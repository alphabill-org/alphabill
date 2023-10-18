package rpc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/gorilla/mux"
)

func getUnit(s *state.State, index keyvaluedb.KeyValueDB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		unitIDString := mux.Vars(r)["unitID"]
		unitID, err := hex.DecodeString(unitIDString)
		if err != nil {
			util.WriteCBORError(w, fmt.Errorf("invalid unit ID: %w", err), http.StatusBadRequest)
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
					util.WriteCBORError(w, fmt.Errorf("invalid fields query parameter: %v", field), http.StatusBadRequest)
					return
				}

			}
			return
		}
		txOrderHashString := strings.TrimSpace(query.Get("txOrderHash"))
		var txOrderHash []byte
		if txOrderHashString != "" {
			h, err := hex.DecodeString(txOrderHashString)
			if err != nil {
				util.WriteCBORError(w, fmt.Errorf("invalid tx order hash format: %w", err), http.StatusBadRequest)
				return
			}
			txOrderHash = h

		}
		fmt.Printf("%X %X %v, %v", txOrderHash, unitID, returnProof, returnUnitData)
		if txOrderHash == nil {
			latestState(w, s.Clone(), unitID, returnProof, returnUnitData)
			return
		}
		key := bytes.Join([][]byte{unitID, txOrderHash}, nil)
		it := index.Find(key)
		defer func() { _ = it.Close() }()
		if !it.Valid() {
			util.WriteCBORError(w, errors.New("not found"), http.StatusNotFound)
			return
		}
		response := &state.UnitDataAndProof{}
		if err := it.Value(response); err != nil {
			util.WriteCBORError(w, err, http.StatusInternalServerError)
			return
		}
		if !returnUnitData {
			response.Data = nil
		}
		if !returnProof {
			response.Proof = nil
		}
		util.WriteCBORResponse(w, response, http.StatusOK)
	}
}

func latestState(w http.ResponseWriter, clonedState *state.State, unitID []byte, returnProof bool, returnData bool) {
	unit, err := clonedState.GetUnit(unitID, true)
	if err != nil {
		// TODO handle not found
		util.WriteCBORError(w, err, http.StatusInternalServerError)
		return
	}
	response := &state.UnitDataAndProof{}
	if returnData {
		response.Data = unit.Data()
	}
	if returnProof {
		p, err := clonedState.CreateUnitStateProof(unitID, len(unit.Logs())-1, nil)
		if err != nil {
			util.WriteCBORError(w, err, http.StatusInternalServerError)
			return
		}
		response.Proof = p
	}
	util.WriteCBORResponse(w, response, http.StatusOK)
}
