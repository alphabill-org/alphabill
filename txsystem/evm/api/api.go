package api

import (
	"log/slog"
	"math/big"
	"net/http"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/gorilla/mux"
)

type API struct {
	state            *state.State
	systemIdentifier types.SystemID
	gasUnitPrice     *big.Int
	blockGasLimit    uint64
	log              *slog.Logger
}

func NewAPI(s *state.State, systemIdentifier types.SystemID, gasUnitPrice *big.Int, blockGasLimit uint64, log *slog.Logger) *API {
	return &API{
		state:            s,
		systemIdentifier: systemIdentifier,
		gasUnitPrice:     gasUnitPrice,
		blockGasLimit:    blockGasLimit,
		log:              log,
	}
}

func (a *API) Register(r *mux.Router) {
	evmRouter := r.PathPrefix("/evm").Subrouter()
	evmRouter.HandleFunc("/call", a.CallEVM).Methods(http.MethodPost, http.MethodOptions)
	evmRouter.HandleFunc("/balance/{address}", a.Balance).Methods(http.MethodGet, http.MethodOptions)
	evmRouter.HandleFunc("/gasPrice", a.GasPrice).Methods(http.MethodGet, http.MethodOptions)
	evmRouter.HandleFunc("/transactionCount/{address}", a.TransactionCount).Methods(http.MethodGet, http.MethodOptions)
}
