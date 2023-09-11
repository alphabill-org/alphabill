package api

import (
	"errors"
	"net/http"

	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
)

func (a *API) Balance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	adr := vars["address"]
	address := common.HexToAddress(adr)
	db := statedb.NewStateDB(a.state.Clone())
	if !db.Exist(address) {
		util.WriteCBORError(w, errors.New("address not found"), http.StatusNotFound)
		return
	}
	balance := db.GetBalance(address)
	util.WriteCBORResponse(w, &struct {
		_       struct{} `cbor:",toarray"`
		Balance string
	}{
		Balance: balance.String(),
	}, http.StatusOK)
}
