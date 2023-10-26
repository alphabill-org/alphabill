package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"

	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
)

const (
	ContentType     = "Content-Type"
	ApplicationJson = "application/json"
	ApplicationCbor = "application/cbor"
	UserAgent       = "User-Agent"

	QueryParamOffsetKey   = "offsetKey"
	QueryParamLimit       = "limit"
	HeaderLink            = "Link"
	HeaderLinkValueFormat = `<%s>; rel="next"`
)

var (
	// ErrInvalidRequest is returned when backend responded with 4nn status code, use errors.Is to check for it.
	ErrInvalidRequest = errors.New("invalid request")

	// ErrNotFound is returned when backend responded with 404 status code, use errors.Is to check for it.
	ErrNotFound = errors.New("not found")
)

type (
	EmptyResponse struct{}

	ErrorResponse struct {
		Message string `json:"message"`
	}

	ResponseWriter struct {
		LogErr func(err error)
	}

	// InfoResponse should be compatible with Node /info request
	InfoResponse struct {
		SystemID string `json:"system_id"` // hex encoded system identifier (without 0x prefix)
		Name     string `json:"name"`      // one of [money backend | tokens backend]
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

func ParseHex[T types.SystemID | types.UnitID | TxHash | []byte](value string, required bool) (T, error) {
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

func SetLinkHeader(u *url.URL, w http.ResponseWriter, next string) {
	if next == "" {
		w.Header().Del(HeaderLink)
		return
	}
	qp := u.Query()
	qp.Set(QueryParamOffsetKey, next)
	u.RawQuery = qp.Encode()
	w.Header().Set(HeaderLink, fmt.Sprintf(HeaderLinkValueFormat, u))
}

/*
parseMaxResponseItems parses input "s" as integer.
When empty string or int over "maxValue" is sent in "maxValue" is returned.
In case of invalid int or value smaller than 1 error is returned.
*/
func ParseMaxResponseItems(s string, maxValue int) (int, error) {
	if s == "" {
		return maxValue, nil
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q as integer: %w", s, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("value must be greater than zero, got %d", v)
	}

	if v > maxValue {
		return maxValue, nil
	}
	return v, nil
}

func GetURL(url url.URL, pathElements ...string) *url.URL {
	url.Path = path.Join(pathElements...)
	return &url
}

func SetPaginationParams(u *url.URL, offset string, limit int) {
	q := u.Query()
	if offset != "" {
		q.Add(QueryParamOffsetKey, offset)
	}
	if limit > 0 {
		q.Add(QueryParamLimit, strconv.Itoa(limit))
	}
	u.RawQuery = q.Encode()
}

var linkHdrMatcher = regexp.MustCompile(`<(.*)>; rel="next"`)

func ExtractOffsetMarker(rsp *http.Response) (string, error) {
	lh := rsp.Header.Get(HeaderLink)
	if lh == "" {
		return "", nil
	}

	match := linkHdrMatcher.FindStringSubmatch(lh)
	if len(match) != 2 {
		return "", fmt.Errorf("link header didn't result in expected match\nHeader: %s\nmatches: %v", lh, match)
	}

	u, err := url.Parse(match[1])
	if err != nil {
		return "", fmt.Errorf("failed to parse Link header as URL: %w", err)
	}
	return u.Query().Get(QueryParamOffsetKey), nil
}

func SetQueryParam(u *url.URL, key, val string) {
	q := u.Query()
	q.Add(key, val)
	u.RawQuery = q.Encode()
}

// DecodeResponse when "rsp" StatusCode is equal to "successStatus" response body is decoded into "data".
// In case of some other response status body is expected to contain error response json struct.
func DecodeResponse(rsp *http.Response, successStatus int, data any, allowEmptyResponse bool) error {
	defer rsp.Body.Close()
	type Decoder interface {
		Decode(val interface{}) error
	}
	var dec Decoder
	contentType := rsp.Header.Get(ContentType)
	if contentType == ApplicationCbor {
		dec = cbor.NewDecoder(rsp.Body)
	} else {
		dec = json.NewDecoder(rsp.Body)
	}
	if rsp.StatusCode == successStatus {
		err := dec.Decode(data)
		if err != nil && (!errors.Is(err, io.EOF) || !allowEmptyResponse) {
			return fmt.Errorf("failed to decode response body: %w", err)
		}
		return nil
	}

	var errResponse ErrorResponse
	if err := json.NewDecoder(rsp.Body).Decode(&errResponse); err != nil {
		return fmt.Errorf("failed to decode error from the response body (%s): %w", rsp.Status, err)
	}

	msg := fmt.Sprintf("backend responded %s: %s", rsp.Status, errResponse.Message)
	switch {
	case rsp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%s: %w", errResponse.Message, ErrNotFound)
	case 400 <= rsp.StatusCode && rsp.StatusCode < 500:
		return fmt.Errorf("%s: %w", msg, ErrInvalidRequest)
	}
	return errors.New(msg)
}
