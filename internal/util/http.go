package util

import (
	"fmt"
	"net/http"

	log "github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/fxamacker/cbor/v2"
)

var logger = log.CreateForPackage()

// WriteCBORResponse replies to the request with the given response and HTTP code.
func WriteCBORResponse(w http.ResponseWriter, response any, statusCode int) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(statusCode)
	if err := cbor.NewEncoder(w).Encode(response); err != nil {
		logger.Warning("Failed to write CBOR response: %v", err)
	}
}

// WriteCBORError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func WriteCBORError(w http.ResponseWriter, e error, code int) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(code)
	if err := cbor.NewEncoder(w).Encode(struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{
		Err: fmt.Sprintf("%v", e),
	}); err != nil {
		logger.Warning("Failed to write CBOR error response: %v", err)
	}
}
