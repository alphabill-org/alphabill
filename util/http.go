package util

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/fxamacker/cbor/v2"
)

// WriteCBORResponse replies to the request with the given response and HTTP code.
func WriteCBORResponse(w http.ResponseWriter, response any, statusCode int, log *slog.Logger) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(statusCode)
	if err := cbor.NewEncoder(w).Encode(response); err != nil {
		log.Warn("failed to write CBOR response", logger.Error(err))
	}
}

// WriteCBORError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func WriteCBORError(w http.ResponseWriter, e error, code int, log *slog.Logger) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(code)
	if err := cbor.NewEncoder(w).Encode(struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{
		Err: fmt.Sprintf("%v", e),
	}); err != nil {
		log.Warn("failed to write CBOR error response", logger.Error(err))
	}
}
