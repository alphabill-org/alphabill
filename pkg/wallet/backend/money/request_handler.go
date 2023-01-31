package money

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/alphabill-org/alphabill/internal/block"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	contentType = "Content-Type"
)

type (
	RequestHandler struct {
		Service            WalletBackendService
		ListBillsPageLimit int
	}

	ListBillsResponse struct {
		Total int           `json:"total"`
		Bills []*ListBillVM `json:"bills"`
	}

	ListBillVM struct {
		Id       []byte `json:"id"`
		Value    uint64 `json:"value"`
		TxHash   []byte `json:"txHash"`
		IsDCBill bool   `json:"isDCBill"`
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance"`
	}

	AddKeyRequest struct {
		Pubkey string `json:"pubkey"`
	}

	BlockHeightResponse struct {
		BlockHeight uint64 `json:"blockHeight"`
	}

	EmptyResponse struct{}

	ErrorResponse struct {
		Message string `json:"message"`
	}
)

var (
	errMissingPubKeyQueryParam = errors.New("missing required pubkey query parameter")
	errInvalidPubKeyLength     = errors.New("pubkey hex string must be 68 characters long (with 0x prefix)")
	errMissingBillIDQueryParam = errors.New("missing required bill_id query parameter")
	errInvalidBillIDLength     = errors.New("bill_id hex string must be 66 characters long (with 0x prefix)")
)

func (s *RequestHandler) Router() *mux.Router {
	// TODO add request/response headers middleware
	apiRouter := mux.NewRouter().StrictSlash(true).PathPrefix("/api").Subrouter()

	// add cors middleware
	// content-type needs to be explicitly defined without this content-type header is not allowed and cors filter is not applied
	// OPTIONS method needs to be explicitly defined for each handler func
	apiRouter.Use(handlers.CORS(handlers.AllowedHeaders([]string{contentType})))

	// version v1 router
	apiV1 := apiRouter.PathPrefix("/v1").Subrouter()
	apiV1.HandleFunc("/list-bills", s.listBillsFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/balance", s.balanceFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/proof", s.getProofFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/block-height", s.blockHeightFunc).Methods("GET", "OPTIONS")
	return apiRouter
}

func (s *RequestHandler) listBillsFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		wlog.Debug("error parsing GET /list-bills request: ", err)
		s.handlePubKeyNotFoundError(w, err)
		return
	}
	bills, err := s.Service.GetBills(pk)
	if err != nil {
		wlog.Error("error on GET /list-bills: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	limit, offset := s.parsePagingParams(r)
	// if offset and limit go out of bounds just return what we have
	if offset > len(bills) {
		offset = len(bills)
	}
	if offset+limit > len(bills) {
		limit = len(bills) - offset
	}
	res := newListBillsResponse(bills, limit, offset)
	writeAsJson(w, res)
}

func (s *RequestHandler) balanceFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		wlog.Debug("error parsing GET /balance request: ", err)
		s.handlePubKeyNotFoundError(w, err)
		return
	}
	bills, err := s.Service.GetBills(pk)
	if err != nil {
		wlog.Error("error on GET /balance: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var sum uint64
	for _, b := range bills {
		if !b.IsDCBill {
			sum += b.Value
		}
	}
	res := &BalanceResponse{Balance: sum}
	writeAsJson(w, res)
}

func (s *RequestHandler) getProofFunc(w http.ResponseWriter, r *http.Request) {
	billID, err := parseBillID(r)
	if err != nil {
		wlog.Debug("error parsing GET /proof request: ", err)
		w.WriteHeader(http.StatusBadRequest)
		if errors.Is(err, errMissingBillIDQueryParam) || errors.Is(err, errInvalidBillIDLength) {
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			writeAsJson(w, ErrorResponse{Message: "invalid bill id format"})
		}
		return
	}
	bill, err := s.Service.GetBill(billID)
	if err != nil {
		wlog.Error("error on GET /proof: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if bill == nil {
		wlog.Debug("error on GET /proof: ", err)
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: "bill does not exist"})
		return
	}
	writeAsProtoJson(w, bill.toProtoBills())
}

func (s *RequestHandler) parsePubkeyURLParam(r *http.Request) ([]byte, error) {
	vars := mux.Vars(r)
	pubkeyParam := vars["pubkey"]
	return parsePubKey(pubkeyParam)
}

func (s *RequestHandler) handlePubKeyNotFoundError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	if errors.Is(err, errMissingPubKeyQueryParam) || errors.Is(err, errInvalidPubKeyLength) {
		writeAsJson(w, ErrorResponse{Message: err.Error()})
	} else {
		writeAsJson(w, ErrorResponse{Message: "invalid pubkey format"})
	}
}

func (s *RequestHandler) readBillsProto(r *http.Request) (*block.Bills, error) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	req := &block.Bills{}
	err = protojson.Unmarshal(b, req)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (s *RequestHandler) blockHeightFunc(w http.ResponseWriter, _ *http.Request) {
	maxBlockNumber, err := s.Service.GetMaxBlockNumber()
	if err != nil {
		log.Err(err).Msg("GET /block-height error fetching max block number")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		writeAsJson(w, &BlockHeightResponse{BlockHeight: maxBlockNumber})
	}
}

func (s *RequestHandler) parsePagingParams(r *http.Request) (int, int) {
	limit := parseInt(r.URL.Query().Get("limit"), s.ListBillsPageLimit)
	if limit < 0 {
		limit = 0
	}
	if limit > s.ListBillsPageLimit {
		limit = s.ListBillsPageLimit
	}
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func writeAsJson(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(res)
	if err != nil {
		wlog.Error("error encoding response to json ", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func writeAsProtoJson(w http.ResponseWriter, res proto.Message) {
	w.Header().Set("Content-Type", "application/json")
	bytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(res)
	if err != nil {
		wlog.Error("error encoding response to proto json: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(bytes)
	if err != nil {
		wlog.Error("error writing proto json to response: ", err)
	}
}

func parsePubKeyQueryParam(r *http.Request) ([]byte, error) {
	return parsePubKey(r.URL.Query().Get("pubkey"))
}

func parsePubKey(pubkey string) ([]byte, error) {
	if pubkey == "" {
		return nil, errMissingPubKeyQueryParam
	}
	return decodePubKeyHex(pubkey)
}

func decodePubKeyHex(pubKey string) ([]byte, error) {
	if len(pubKey) != 68 {
		return nil, errInvalidPubKeyLength
	}
	bytes, err := hexutil.Decode(pubKey)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func parseBillID(r *http.Request) ([]byte, error) {
	billIdHex := r.URL.Query().Get("bill_id")
	if billIdHex == "" {
		return nil, errMissingBillIDQueryParam
	}
	return decodeBillIdHex(billIdHex)
}

func decodeBillIdHex(billID string) ([]byte, error) {
	if len(billID) != 66 {
		return nil, errInvalidBillIDLength
	}
	billIdBytes, err := hexutil.Decode(billID)
	if err != nil {
		return nil, err
	}
	return billIdBytes, nil
}

func parseInt(str string, def int) int {
	num, err := strconv.Atoi(str)
	if err != nil {
		return def
	}
	return num
}

func newListBillsResponse(bills []*Bill, limit, offset int) *ListBillsResponse {
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].OrderNumber < bills[j].OrderNumber
	})
	billVMs := toBillVMList(bills)
	return &ListBillsResponse{Bills: billVMs[offset : offset+limit], Total: len(bills)}
}

func toBillVMList(bills []*Bill) []*ListBillVM {
	billVMs := make([]*ListBillVM, len(bills))
	for i, b := range bills {
		billVMs[i] = &ListBillVM{
			Id:       b.Id,
			Value:    b.Value,
			TxHash:   b.TxHash,
			IsDCBill: b.IsDCBill,
		}
	}
	return billVMs
}
