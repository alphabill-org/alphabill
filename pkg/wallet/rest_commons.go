package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
)

const (
	ContentType     = "Content-Type"
	ApplicationJson = "application/json"
	ApplicationCbor = "application/cbor"
)

type (
	EmptyResponse struct{}

	ErrorResponse struct {
		Message string `json:"message"`
	}

	ResponseWriter struct {
		LogErr func(a ...any)
	}
)

var ErrRecordNotFound = errors.New("not found")

func (rw *ResponseWriter) logError(err error) {
	if rw.LogErr != nil {
		rw.LogErr(err)
	}
}

func (rw *ResponseWriter) WriteResponse(w http.ResponseWriter, data any) {
	w.Header().Set(ContentType, ApplicationJson)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		rw.logError(fmt.Errorf("failed to encode response data as json: %w", err))
	}
}

func (rw *ResponseWriter) WriteCborResponse(w http.ResponseWriter, data any) {
	w.Header().Set(ContentType, ApplicationCbor)
	if err := cbor.NewEncoder(w).Encode(data); err != nil {
		rw.logError(fmt.Errorf("failed to encode response data as cbor: %w", err))
	}
}

func (rw *ResponseWriter) WriteErrorResponse(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrRecordNotFound) {
		rw.ErrorResponse(w, http.StatusNotFound, err)
		return
	}

	rw.ErrorResponse(w, http.StatusInternalServerError, err)
	rw.logError(err)
}

func (rw *ResponseWriter) InvalidParamResponse(w http.ResponseWriter, name string, err error) {
	rw.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid parameter %q: %w", name, err))
}

func (rw *ResponseWriter) ErrorResponse(w http.ResponseWriter, code int, err error) {
	w.Header().Set(ContentType, ApplicationJson)
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Message: err.Error()}); err != nil {
		rw.logError(fmt.Errorf("failed to encode error response as json: %w", err))
	}
}

func ParsePubKey(pubkey string, required bool) (PubKey, error) {
	if pubkey == "" {
		if required {
			return nil, fmt.Errorf("parameter is required")
		}
		return nil, nil
	}
	return DecodePubKeyHex(pubkey)
}

func DecodePubKeyHex(pubKey string) (PubKey, error) {
	if n := len(pubKey); n != 68 {
		s := " starting "
		switch {
		case n == 0:
			s = ""
		case n <= 6:
			s += pubKey
		default:
			s += pubKey[:6]
		}
		return nil, fmt.Errorf("must be 68 characters long (including 0x prefix), got %d characters%s", len(pubKey), s)
	}
	bytes, err := hexutil.Decode(pubKey)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func ParseHex[T UnitID | TxHash](value string, required bool) (T, error) {
	if value == "" {
		if required {
			return nil, fmt.Errorf("parameter is required")
		}
		return nil, nil
	}

	bytes, err := hexutil.Decode(value)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func EncodeHex(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return hexutil.Encode(value)
}
