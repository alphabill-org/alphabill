package rpc

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
)

const (
	headerContentType = "Content-Type"
	applicationJson   = "application/json"
	applicationCBOR   = "application/cbor"

	metricsScopeRESTAPI = "rest_api"
	MetricsScopeGRPCAPI = "grpc_api"
)

var allowedCORSHeaders = []string{"Accept", "Accept-Language", "Content-Language", "Origin", headerContentType}

type (
	// Registrar registers new HTTP handlers for given router.
	Registrar interface {
		Register(r *mux.Router)
	}

	// RegistrarFunc type is an adapter to allow the use of ordinary function as Registrar.
	RegistrarFunc func(r *mux.Router)

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		PrometheusRegisterer() prometheus.Registerer
	}
)

func NewRESTServer(addr string, maxBodySize int64, obs Observability, log *slog.Logger, registrars ...Registrar) *http.Server {
	mtr := obs.Meter(metricsScopeRESTAPI)

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(http.NotFound)
	apiV1Router := r.PathPrefix("/api/v1").Subrouter()
	apiV1Router.Use(handlers.CORS(handlers.AllowedHeaders(allowedCORSHeaders)), instrumentHTTP(mtr, log))

	for _, registrar := range registrars {
		registrar.Register(apiV1Router)
	}

	return &http.Server{
		Addr:              addr,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		Handler:           http.MaxBytesHandler(r, maxBodySize),
	}
}

func (f RegistrarFunc) Register(r *mux.Router) {
	f(r)
}
