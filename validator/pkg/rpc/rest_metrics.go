package rpc

import (
	"net/http"

	"github.com/alphabill-org/alphabill/validator/pkg/metrics"
	"github.com/gorilla/mux"
)

func MetricsEndpoints() RegistrarFunc {
	return func(r *mux.Router) {
		if metrics.Enabled() {
			r.Handle("/metrics", metrics.PrometheusHandler()).Methods(http.MethodGet, http.MethodOptions)
		}
	}
}
