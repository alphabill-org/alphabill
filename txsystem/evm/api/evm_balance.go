package api

import (
	"errors"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"

	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
)

func (a *API) Balance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]
	address := common.HexToAddress(addr)
	db := statedb.NewStateDB(a.state.Clone(), a.log)
	if !db.Exist(address) {
		WriteCBORError(w, errors.New("address not found"), http.StatusNotFound, a.log)
		return
	}
	balance := db.GetBalance(address)
	abData := db.GetAlphaBillData(address)

	WriteCBORResponse(w, &struct {
		_       struct{} `cbor:",toarray"`
		Balance string
		Counter uint64
	}{
		Balance: balance.String(),
		Counter: abData.GetCounter(),
	}, http.StatusOK, a.log)
}
