package money

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	_ "github.com/alphabill-org/alphabill/pkg/wallet/backend/money/docs"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
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
		Total int           `json:"total" example:"1"`
		Bills []*ListBillVM `json:"bills"`
	}

	ListBillVM struct {
		Id       []byte `json:"id" swaggertype:"string" format:"base64" example:"AAAAAAgwv3UA1HfGO4qc1T3I3EOvqxfcrhMjJpr9Tn4="`
		Value    uint64 `json:"value,string" example:"1000"`
		TxHash   []byte `json:"txHash" swaggertype:"string" format:"base64" example:"Q4ShCITC0ODXPR+j1Zl/teYcoU3/mAPy0x8uSsvQFM8="`
		IsDCBill bool   `json:"isDcBill" example:"false"`
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance,string"`
	}

	AddKeyRequest struct {
		Pubkey string `json:"pubkey"`
	}

	RoundNumberResponse struct {
		RoundNumber uint64 `json:"roundNumber,string"`
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
	router := mux.NewRouter().StrictSlash(true)

	apiRouter := router.PathPrefix("/api").Subrouter()
	// add cors middleware
	// content-type needs to be explicitly defined without this content-type header is not allowed and cors filter is not applied
	// OPTIONS method needs to be explicitly defined for each handler func
	apiRouter.Use(handlers.CORS(handlers.AllowedHeaders([]string{contentType})))

	// version v1 router
	apiV1 := apiRouter.PathPrefix("/v1").Subrouter()
	apiV1.HandleFunc("/list-bills", s.listBillsFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/balance", s.balanceFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/proof", s.getProofFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/round-number", s.blockHeightFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/fee-credit-bill", s.getFeeCreditBillFunc).Methods("GET", "OPTIONS")

	apiV1.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("/api/v1/swagger/doc.json"), //The url pointing to API definition
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	)).Methods(http.MethodGet)

	return router
}

// @Summary List bills
// @Id 1
// @version 1.0
// @produce application/json
// @Param pubkey query string true "Public key prefixed with 0x" example(0x000000000000000000000000000000000000000000000000000000000000000123)
// @Param limit query int false "limits how many bills are returned in response" default(100)
// @Param offset query int false "response will include bills starting after offset" default(0)
// @Success 200 {object} ListBillsResponse
// @Failure 400 {object} money.ErrorResponse
// @Failure 500
// @Router /list-bills [get]
func (s *RequestHandler) listBillsFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		log.Debug("error parsing GET /list-bills request: ", err)
		s.handlePubKeyNotFoundError(w, err)
		return
	}
	includeDCBills, err := parseIncludeDCBillsQueryParam(r, true)
	if err != nil {
		log.Debug("error parsing GET /balance request: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	bills, err := s.Service.GetBills(pk)
	if err != nil {
		log.Error("error on GET /list-bills: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !includeDCBills {
		for i, b := range bills {
			if b.IsDCBill {
				bills = append(bills[:i], bills[i+1:]...)
			}
		}
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

// @Summary Get balance
// @Id 2
// @version 1.0
// @produce application/json
// @Param pubkey query string true "Public key prefixed with 0x"
// @Success 200 {object} BalanceResponse
// @Failure 400 {object} money.ErrorResponse
// @Failure 500
// @Router /balance [get]
func (s *RequestHandler) balanceFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		log.Debug("error parsing GET /balance request: ", err)
		s.handlePubKeyNotFoundError(w, err)
		return
	}
	includeDCBills, err := parseIncludeDCBillsQueryParam(r, false)
	if err != nil {
		log.Debug("error parsing GET /balance request: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	bills, err := s.Service.GetBills(pk)
	if err != nil {
		log.Error("error on GET /balance: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var sum uint64
	for _, b := range bills {
		if !b.IsDCBill || includeDCBills {
			sum += b.Value
		}
	}
	res := &BalanceResponse{Balance: sum}
	writeAsJson(w, res)
}

// @Summary Get proof
// @Id 3
// @version 1.0
// @produce application/json
// @Param bill_id query string true "ID of the bill (hex)"
// @Success 200 {object} bp.Bills
// @Failure 400 {object} money.ErrorResponse
// @Failure 404 {object} money.ErrorResponse
// @Failure 500
// @Router /proof [get]
func (s *RequestHandler) getProofFunc(w http.ResponseWriter, r *http.Request) {
	billID, err := parseBillID(r)
	if err != nil {
		log.Debug("error parsing GET /proof request: ", err)
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
		log.Error("error on GET /proof: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if bill == nil {
		log.Debug("error on GET /proof: ", err)
		w.WriteHeader(http.StatusNotFound)
		writeAsJson(w, ErrorResponse{Message: "bill does not exist"})
		return
	}
	writeAsProtoJson(w, bill.toProtoBills())
}

func (s *RequestHandler) handlePubKeyNotFoundError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	if errors.Is(err, errMissingPubKeyQueryParam) || errors.Is(err, errInvalidPubKeyLength) {
		writeAsJson(w, ErrorResponse{Message: err.Error()})
	} else {
		writeAsJson(w, ErrorResponse{Message: "invalid pubkey format"})
	}
}

func (s *RequestHandler) readBillsProto(r *http.Request) (*bp.Bills, error) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	req := &bp.Bills{}
	err = protojson.Unmarshal(b, req)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// @Summary Money partition's latest block number
// @Id 4
// @version 1.0
// @produce application/json
// @Success 200 {object} RoundNumberResponse
// @Router /round-number [get]
func (s *RequestHandler) blockHeightFunc(w http.ResponseWriter, r *http.Request) {
	lastRoundNumber, err := s.Service.GetRoundNumber(r.Context())
	if err != nil {
		log.Error("GET /round-number error fetching round number", err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		writeAsJson(w, &RoundNumberResponse{RoundNumber: lastRoundNumber})
	}
}

// @Summary Get Fee Credit Bill
// @Id 5
// @version 1.0
// @produce application/json
// @Param bill_id query string true "ID of the bill (hex)"
// @Success 200 {object} bp.Bill
// @Router /fee-credit-bill [get]
func (s *RequestHandler) getFeeCreditBillFunc(w http.ResponseWriter, r *http.Request) {
	billID, err := parseBillID(r)
	if err != nil {
		log.Debug("error parsing GET /fee-credit-bill request: ", err)
		w.WriteHeader(http.StatusBadRequest)
		if errors.Is(err, errMissingBillIDQueryParam) || errors.Is(err, errInvalidBillIDLength) {
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			writeAsJson(w, ErrorResponse{Message: "invalid bill_id format"})
		}
		return
	}
	fcb, err := s.Service.GetFeeCreditBill(billID)
	if err != nil {
		log.Error("error on GET /fee-credit-bill: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if fcb == nil {
		log.Debug("error on GET /fee-credit-bill: ", err)
		w.WriteHeader(http.StatusNotFound)
		writeAsJson(w, ErrorResponse{Message: "fee credit bill does not exist"})
		return
	}
	writeAsProtoJson(w, fcb.toProto())
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
		log.Error("error encoding response to json ", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func writeAsProtoJson(w http.ResponseWriter, res proto.Message) {
	w.Header().Set("Content-Type", "application/json")
	bytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(res)
	if err != nil {
		log.Error("error encoding response to proto json: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(bytes)
	if err != nil {
		log.Error("error writing proto json to response: ", err)
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

func parseIncludeDCBillsQueryParam(r *http.Request, defaultValue bool) (bool, error) {
	if r.URL.Query().Has("includedcbills") {
		return strconv.ParseBool(r.URL.Query().Get("includedcbills"))
	}
	return defaultValue, nil
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
