package rpc

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func MetricsEndpoints(pr prometheus.Registerer) RegistrarFunc {
	if pr == nil {
		return func(r *mux.Router) { /* NOP */ }
	}
	return func(r *mux.Router) {
		r.Handle("/metrics", promhttp.HandlerFor(pr.(prometheus.Gatherer), promhttp.HandlerOpts{MaxRequestsInFlight: 1})).Methods(http.MethodGet, http.MethodOptions)
	}
}
