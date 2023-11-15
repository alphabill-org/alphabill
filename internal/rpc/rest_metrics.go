package rpc

import (
	"net/http"

	"github.com/gorilla/mux"
)

func MetricsEndpoints(h http.Handler) RegistrarFunc {
	return func(r *mux.Router) {
		if h != nil {
			r.Handle("/metrics", h).Methods(http.MethodGet, http.MethodOptions)
		}
	}
}
