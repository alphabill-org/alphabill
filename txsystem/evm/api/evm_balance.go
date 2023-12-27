package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem/evm/conversion"
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
	rpc.WriteCBORResponse(w, &struct {
		_       struct{} `cbor:",toarray"`
		Balance string
	}{
		Balance: obj.Account.Balance.String(),
	}, http.StatusOK, a.log)
}

func (a *API) FeeCreditInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitStr := vars["unitID"]
	unitID, err := hex.DecodeString(unitStr)
	if err != nil {
		rpc.WriteCBORError(w, errors.New("invalid unit ID"), http.StatusBadRequest, a.log)
		return
	}
	u, err := a.state.GetUnit(unitID, false)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			rpc.WriteCBORError(w, errors.New("unit not found"), http.StatusNotFound, a.log)
			return
		}
		rpc.WriteCBORError(w, fmt.Errorf("unit load failed: %w", err), http.StatusInternalServerError, a.log)
		return
	}
	obj := u.Data().(*unit.StateObject)

	rpc.WriteCBORResponse(w, &struct {
		_        struct{} `cbor:",toarray"`
		Balance  uint64
		Backlink []byte
		Timeout  uint64
		Locked   uint64
	}{
		Balance:  conversion.WeiToAlpha(obj.Account.Balance),
		Backlink: obj.Account.Backlink,
		Timeout:  obj.Account.Timeout,
		Locked:   obj.Account.Locked,
	}, http.StatusOK, a.log)
}
