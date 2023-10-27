package rpc

import (
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	headerContentType = "Content-Type"
	applicationJson   = "application/json"
	applicationCBOR   = "application/cbor"
)

var allowedCORSHeaders = []string{"Accept", "Accept-Language", "Content-Language", "Origin", headerContentType}

type (

	// Registrar registers new HTTP handlers for given router.
	Registrar interface {
		Register(r *mux.Router)
	}

	// RegistrarFunc type is an adapter to allow the use of ordinary function as Registrar.
	RegistrarFunc func(r *mux.Router)
)

func NewRESTServer(addr string, maxBodySize int64, registrars ...Registrar) *http.Server {
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(http.NotFound)
	apiV1Router := r.PathPrefix("/api/v1").Subrouter()
	apiV1Router.Use(handlers.CORS(handlers.AllowedHeaders(allowedCORSHeaders)))

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
