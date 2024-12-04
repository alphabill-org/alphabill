package rpc

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
)

const (
	headerContentType = "Content-Type"

	metricsScopeJRPCAPI = "jrpc_api" // json-rpc
	metricsScopeRESTAPI = "rest_api"

	DefaultMaxBodyBytes           int64 = 4194304 // 4MB
	DefaultBatchItemLimit         int   = 1000
	DefaultBatchResponseSizeLimit int   = int(DefaultMaxBodyBytes)
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
		Logger() *slog.Logger
	}

	API struct {
		Namespace string
		Service   interface{}
	}

	// ServerConfiguration is a common configuration for RPC servers.
	ServerConfiguration struct {
		// Address specifies the TCP address for the server to listen on, in the form "host:port".
		// REST server isn't initialised if Address is empty.
		Address string

		// ReadTimeout is the maximum duration for reading the entire request, including the body. A zero or negative
		// value means there will be no timeout.
		ReadTimeout time.Duration

		// ReadHeaderTimeout is the amount of time allowed to read request headers. If ReadHeaderTimeout is zero, the
		// value of ReadTimeout is used. If both are zero, there is no timeout.
		ReadHeaderTimeout time.Duration

		// WriteTimeout is the maximum duration before timing out writes of the response. A zero or negative value means
		// there will be no timeout.
		WriteTimeout time.Duration

		// IdleTimeout is the maximum amount of time to wait for the next request when keep-alive is enabled. If
		// IdleTimeout is zero, the value of ReadTimeout is used. If both are zero, there is no timeout.
		IdleTimeout time.Duration

		// MaxHeaderBytes controls the maximum number of bytes the server will read parsing the request header's keys
		// and values, including the request line. It does not limit the size of the request body. If zero,
		// http.DefaultMaxHeaderBytes is used.
		MaxHeaderBytes int

		// MaxHeaderBytes controls the maximum number of bytes the server will read parsing the request body. If zero,
		// MaxBodyBytes is used.
		MaxBodyBytes int64

		// BatchItemLimit is the maximum number of requests in a batch.
		BatchItemLimit int

		// BatchResponseSizeLimit is the maximum number of response bytes across all requests in a batch.
		BatchResponseSizeLimit int

		Router Registrar

		// APIs contains is an array of enabled RPC services.
		APIs []API
	}
)

func (c *ServerConfiguration) IsAddressEmpty() bool {
	return strings.TrimSpace(c.Address) == ""
}

func NewHTTPServer(conf *ServerConfiguration, obs Observability, registrars ...Registrar) (*http.Server, error) {
	restMeter := obs.Meter(metricsScopeRESTAPI)

	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(http.NotFound)
	restRouter := router.PathPrefix("/api/v1").Subrouter()
	restRouter.Use(
		handlers.CORS(handlers.AllowedHeaders(allowedCORSHeaders)),
		instrumentHTTP(restMeter, obs.Logger()))
	for _, registrar := range registrars {
		registrar.Register(restRouter)
	}

	rpcServer := rpc.NewServer()
	rpcServer.SetBatchLimits(conf.BatchItemLimit, conf.BatchResponseSizeLimit)

	for _, api := range conf.APIs {
		if err := rpcServer.RegisterName(api.Namespace, api.Service); err != nil {
			return nil, fmt.Errorf("failed to register API: %w", err)
		}
	}

	// RPC WebSocket handler
	router.Handle("/rpc", rpcServer.WebsocketHandler([]string{"*"})).Headers(
		"Connection", "Upgrade",
		"Upgrade", "websocket",
	)

	// RPC HTTP handler
	rpcRouter := router.PathPrefix("/rpc").Subrouter()
	rpcRouter.Handle("", rpcServer)
	rpcRouter.Use(handlers.CORS(handlers.AllowedHeaders(allowedCORSHeaders)))

	return &http.Server{
		Addr:              conf.Address,
		ReadTimeout:       conf.ReadTimeout,
		ReadHeaderTimeout: conf.ReadHeaderTimeout,
		WriteTimeout:      conf.WriteTimeout,
		IdleTimeout:       conf.IdleTimeout,
		Handler:           http.MaxBytesHandler(router, conf.MaxBodyBytes),
	}, nil
}

// Deprecated: Moving away from REST API to JSON-RPC API, use NewHTTPServer instead.
func NewRESTServer(addr string, maxBodySize int64, obs Observability, registrars ...Registrar) *http.Server {
	conf := &ServerConfiguration{
		Address:      addr,
		MaxBodyBytes: maxBodySize,
	}
	// Ignoring error to preserve the interface for now. The
	// function will be removed once the move to JSON-RPC APIs is
	// complete.
	server, _ := NewHTTPServer(conf, obs, registrars...)
	return server
}

func (f RegistrarFunc) Register(r *mux.Router) {
	f(r)
}
