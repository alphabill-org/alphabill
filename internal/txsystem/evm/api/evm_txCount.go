package api

import (
	"errors"
	"net/http"

	"github.com/alphabill-org/alphabill/validator/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
)

func (a *API) TransactionCount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	adr := vars["address"]
	address := common.HexToAddress(adr)
	db := statedb.NewStateDB(a.state.Clone(), a.log)
	if !db.Exist(address) {
		util.WriteCBORError(w, errors.New("address not found"), http.StatusNotFound, a.log)
		return
	}
	util.WriteCBORResponse(w, &struct {
		_     struct{} `cbor:",toarray"`
		Nonce uint64
	}{
		Nonce: db.GetNonce(address),
	}, http.StatusOK, a.log)
}
