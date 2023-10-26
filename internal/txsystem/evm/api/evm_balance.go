package api

import (
	"errors"
	"net/http"

	"github.com/alphabill-org/alphabill/validator/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
)

func (a *API) Balance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]
	address := common.HexToAddress(addr)
	db := statedb.NewStateDB(a.state.Clone(), a.log)
	if !db.Exist(address) {
		util.WriteCBORError(w, errors.New("address not found"), http.StatusNotFound, a.log)
		return
	}
	balance := db.GetBalance(address)
	abData := db.GetAlphaBillData(address)
	var backlink []byte
	if abData != nil {
		backlink = abData.TxHash
	}

	util.WriteCBORResponse(w, &struct {
		_        struct{} `cbor:",toarray"`
		Balance  string
		Backlink []byte
	}{
		Balance:  balance.String(),
		Backlink: backlink,
	}, http.StatusOK, a.log)
}
