package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem/evm/unit"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
)

func (a *API) Balance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]
	unitID := unit.NewEvmAccountIDFromAddress(common.HexToAddress(addr))
	u, err := a.state.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			rpc.WriteCBORError(w, errors.New("address not found"), http.StatusNotFound, a.log)
			return
		}
		rpc.WriteCBORError(w, fmt.Errorf("unit load failed: %w", err), http.StatusInternalServerError, a.log)
		return
	}
	obj := u.Data().(*unit.StateObject)
	balance := obj.Account.Balance
	backlink := obj.Account.Backlink

	rpc.WriteCBORResponse(w, &struct {
		_        struct{} `cbor:",toarray"`
		Balance  string
		Backlink []byte
	}{
		Balance:  balance.String(),
		Backlink: backlink,
	}, http.StatusOK, a.log)
}
