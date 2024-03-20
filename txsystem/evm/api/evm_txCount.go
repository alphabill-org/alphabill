package api

import (
	"errors"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"

	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
)

func (a *API) TransactionCount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	adr := vars["address"]
	address := common.HexToAddress(adr)
	db := statedb.NewStateDB(a.state.Clone(), a.log)
	if !db.Exist(address) {
		WriteCBORError(w, errors.New("address not found"), http.StatusNotFound, a.log)
		return
	}
	WriteCBORResponse(w, &struct {
		_     struct{} `cbor:",toarray"`
		Nonce uint64
	}{
		Nonce: db.GetNonce(address),
	}, http.StatusOK, a.log)
}
