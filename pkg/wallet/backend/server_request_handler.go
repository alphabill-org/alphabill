package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	contentType = "Content-Type"
)

type (
	RequestHandler struct {
		service            WalletBackendService
		listBillsPageLimit int
	}

	ListBillsResponse struct {
		Total int       `json:"total"`
		Bills []*BillVM `json:"bills"`
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance"`
	}

	BlockProofResponse struct {
		BillId      string                `json:"billId"`
		BlockNumber uint64                `json:"blockNumber"`
		Tx          *txsystem.Transaction `json:"tx"`
		BlockProof  *block.BlockProof     `json:"blockProof"`
	}

	AddBlockProofRequest struct {
		Pubkey []byte `json:"pubkey" validate:"required,len=33"`
		Bill   *Bill  `json:"bill" validate:"required"`
	}

	AddKeyRequest struct {
		Pubkey string `json:"pubkey"`
	}

	EmptyResponse struct{}

	ErrorResponse struct {
		Message string `json:"message"`
	}

	BillDTO struct {
		Id     []byte `json:"id" validate:"required"`
		Value  uint64 `json:"value" validate:"required"`
		TxHash []byte `json:"txHash" validate:"required"`
		// OrderNumber insertion order of given bill in pubkey => list of bills bucket, needed for determistic paging
		OrderNumber uint64      `json:"orderNumber"`
		BlockProof  *BlockProof `json:"blockProof,omitempty" validate:"required"`
	}

	BillVM struct {
		Id    string `json:"id"`
		Value uint64 `json:"value"`
		// TODO return tx hash here or in block proof?
		// TODO or remove tx hash from proof output?
	}
)

var (
	errMissingPubKeyQueryParam = errors.New("missing required pubkey query parameter")
	errInvalidPubKeyLength     = errors.New("pubkey hex string must be 68 characters long (with 0x prefix)")
	errMissingBillIDQueryParam = errors.New("missing required bill_id query parameter")
	errInvalidBillIDLength     = errors.New("bill_id hex string must be 66 characters long (with 0x prefix)")
)

func (s *RequestHandler) router() *mux.Router {
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
	apiV1.HandleFunc("/block-proof", s.getBlockProofFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/block-proof", s.setBlockProofFunc).Methods("POST", "OPTIONS")

	// TODO authorization
	v1Admin := apiV1.PathPrefix("/admin/").Subrouter()
	v1Admin.HandleFunc("/add-key", s.addKeyFunc).Methods("POST", "OPTIONS")

	return apiRouter
}

func (s *RequestHandler) listBillsFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		wlog.Debug("error parsing GET /list-bills request: ", err)
		w.WriteHeader(http.StatusBadRequest)
		if errors.Is(err, errMissingPubKeyQueryParam) || errors.Is(err, errInvalidPubKeyLength) {
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			writeAsJson(w, ErrorResponse{Message: "invalid pubkey format"})
		}
		return
	}
	bills, err := s.service.GetBills(pk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		wlog.Error("error on GET /list-bills: ", err)
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
		w.WriteHeader(http.StatusBadRequest)
		if errors.Is(err, errMissingPubKeyQueryParam) || errors.Is(err, errInvalidPubKeyLength) {
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			writeAsJson(w, ErrorResponse{Message: "invalid pubkey format"})
		}
		return
	}
	bills, err := s.service.GetBills(pk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		wlog.Error("error on GET /balance: ", err)
		return
	}
	sum := uint64(0)
	for _, b := range bills {
		sum += b.Value
	}
	res := &BalanceResponse{Balance: sum}
	writeAsJson(w, res)
}

func (s *RequestHandler) getBlockProofFunc(w http.ResponseWriter, r *http.Request) {
	billId, err := parseBillId(r)
	if err != nil {
		wlog.Debug("error parsing GET /block-proof request: ", err)
		w.WriteHeader(http.StatusBadRequest)
		if errors.Is(err, errMissingBillIDQueryParam) || errors.Is(err, errInvalidBillIDLength) {
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			writeAsJson(w, ErrorResponse{Message: "invalid bill id format"})
		}
		return
	}
	p, err := s.service.GetBlockProof(billId)
	if err != nil {
		wlog.Error("error on GET /block-proof: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if p == nil {
		wlog.Debug("GET /block-proof does not exist ", fmt.Sprintf("%X\n", billId))
		w.WriteHeader(400)
		writeAsJson(w, ErrorResponse{Message: "block proof does not exist for given bill id"})
		return
	}
	res := newBlockProofResponse(p)
	writeAsJson(w, res)
}

func (s *RequestHandler) setBlockProofFunc(w http.ResponseWriter, r *http.Request) {
	req := &AddBlockProofRequest{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: "invalid request body"})
		wlog.Debug("error decoding POST /block-proof request: ", err)
		return
	}
	err = validate.Struct(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		wlog.Debug("validation error POST /block-proof request: ", err)
		return
	}
	// TODO add pubkey to tracked keys? or return error if given pubkey is not indexed or
	err = s.service.AddBillWithProof(req.Pubkey, req.Bill)
	if err != nil {
		wlog.Error("error on POST /block-proof: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeAsJson(w, EmptyResponse{})
}

func (s *RequestHandler) addKeyFunc(w http.ResponseWriter, r *http.Request) {
	req := &AddKeyRequest{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: "invalid request body"})
		wlog.Debug("error decoding POST /add-key request ", err)
		return
	}
	pubkeyBytes, err := decodePubKeyHex(req.Pubkey)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		wlog.Debug("error parsing POST /add-key request ", err)
		return
	}
	err = s.service.AddKey(pubkeyBytes)
	if err != nil {
		if errors.Is(err, ErrKeyAlreadyExists) {
			wlog.Info("error on POST /add-key key ", req.Pubkey, " already exists")
			w.WriteHeader(http.StatusBadRequest)
			writeAsJson(w, ErrorResponse{Message: "pubkey already exists"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			wlog.Error("error on POST /add-key ", err)
		}
		return
	}
	writeAsJson(w, EmptyResponse{})
}

func (s *RequestHandler) parsePagingParams(r *http.Request) (int, int) {
	limit := parseInt(r.URL.Query().Get("limit"), s.listBillsPageLimit)
	if limit < 0 {
		limit = 0
	}
	if limit > s.listBillsPageLimit {
		limit = s.listBillsPageLimit
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

func parseBillId(r *http.Request) ([]byte, error) {
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
	if str == "" {
		return def
	}
	num, err := strconv.Atoi(str)
	if err != nil {
		return def
	}
	return num
}

func newBlockProofResponse(p *BlockProof) *BlockProofResponse {
	return &BlockProofResponse{
		BillId:      hexutil.Encode(p.BillId),
		BlockNumber: p.BlockNumber,
		Tx:          p.Tx,
		BlockProof:  p.Proof,
	}
}

func newListBillsResponse(bills []*Bill, limit, offset int) *ListBillsResponse {
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].OrderNumber < bills[j].OrderNumber
	})
	billVMs := toBillVMList(bills)
	return &ListBillsResponse{Bills: billVMs[offset : offset+limit], Total: len(bills)}
}

func toBillVMList(bills []*Bill) []*BillVM {
	billVMs := make([]*BillVM, len(bills))
	for i, b := range bills {
		billVMs[i] = &BillVM{
			Id:    hexutil.Encode(b.Id),
			Value: b.Value,
		}
	}
	return billVMs
}
