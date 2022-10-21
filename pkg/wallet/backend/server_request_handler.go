package backend

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/holiman/uint256"
)

type (
	RequestHandler struct {
		service            WalletBackendService
		listBillsPageLimit int
	}

	ListBillsResponse struct {
		Total int     `json:"total"`
		Bills []*Bill `json:"bills"`
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance"`
	}

	BlockProofResponse struct {
		BlockProof *BlockProof `json:"blockProof"`
	}

	AddKeyRequest struct {
		Pubkey string `json:"pubkey"`
	}

	AddKeyResponse struct {
	}

	ErrorResponse struct {
		Message string `json:"message"`
	}
)

var (
	errMissingPubKeyQueryParam = errors.New("missing required pubkey query parameter")
	errInvalidPubKeyLength     = errors.New("pubkey hex string must be 68 characters long (with 0x prefix)")
	errInvalidPubKeyFormat     = errors.New("invalid pubkey format")
	errMissingBillIDQueryParam = errors.New("missing required bill_id query parameter")
	errInvalidBillIDFormat     = errors.New("bill_id must be in hex format with prefix 0x, not have any leading zeros and be max of 32 bytes in size")
)

func (s *RequestHandler) router() *mux.Router {
	// TODO add request/response headers middleware
	r := mux.NewRouter().StrictSlash(true)
	r.HandleFunc("/list-bills", s.listBillsFunc).Methods("GET")
	r.HandleFunc("/balance", s.balanceFunc).Methods("GET")
	r.HandleFunc("/block-proof", s.blockProofFunc).Methods("GET")

	// TODO authorization
	ra := r.PathPrefix("/admin/").Subrouter()
	ra.HandleFunc("/add-key", s.addKeyFunc).Methods("POST")

	return r
}

func (s *RequestHandler) listBillsFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKey(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		wlog.Debug("error parsing GET /list-bills request ", err)
		return
	}
	bills, err := s.service.GetBills(pk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		wlog.Error("error on GET /list-bills ", err)
		return
	}
	limit, offset := s.parsePagingParams(r)
	// if offset and data go out of bounds just return what we have
	if offset > len(bills) {
		offset = len(bills)
	}
	if offset+limit > len(bills) {
		limit = len(bills) - offset
	}
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].OrderNumber < bills[j].OrderNumber
	})
	res := &ListBillsResponse{Bills: bills[offset : offset+limit], Total: len(bills)}
	writeAsJson(w, res)
}

func (s *RequestHandler) balanceFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKey(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		wlog.Debug("error parsing GET /balance request ", err)
		return
	}
	bills, err := s.service.GetBills(pk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		wlog.Error("error on GET /balance ", err)
		return
	}
	sum := uint64(0)
	for _, b := range bills {
		sum += b.Value
	}
	res := &BalanceResponse{Balance: sum}
	writeAsJson(w, res)
}

func (s *RequestHandler) blockProofFunc(w http.ResponseWriter, r *http.Request) {
	billId, err := parseBillId(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		wlog.Debug("error parsing GET /block-proof request ", err)
		return
	}
	p, err := s.service.GetBlockProof(billId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		wlog.Error("error on GET /block-proof ", err)
		return
	}
	res := &BlockProofResponse{p}
	writeAsJson(w, res)
}

func (s *RequestHandler) addKeyFunc(w http.ResponseWriter, r *http.Request) {
	req := &AddKeyRequest{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: "invalid request body"})
		wlog.Debug("error decoding GET /add-key request ", err)
		return
	}
	pubkeyBytes, err := decodePubKeyHex(req.Pubkey)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		wlog.Debug("error parsing GET /balance request ", err)
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
	writeAsJson(w, &AddKeyResponse{})
}

func writeAsJson(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func parseBillId(r *http.Request) ([]byte, error) {
	billIdHex := r.URL.Query().Get("bill_id")
	if billIdHex == "" {
		return nil, errMissingBillIDQueryParam
	}
	billId256, err := uint256.FromHex(billIdHex)
	if err != nil {
		return nil, errInvalidBillIDFormat
	}
	billIdBytes := billId256.Bytes32()
	return billIdBytes[:], nil
}

func parsePubKey(r *http.Request) ([]byte, error) {
	pubKey := r.URL.Query().Get("pubkey")
	if pubKey == "" {
		return nil, errMissingPubKeyQueryParam
	}
	return decodePubKeyHex(pubKey)
}

func decodePubKeyHex(pubKey string) ([]byte, error) {
	if len(pubKey) != 68 {
		return nil, errInvalidPubKeyLength
	}
	bytes, err := hexutil.Decode(pubKey)
	if err != nil {
		return nil, errInvalidPubKeyFormat
	}
	return bytes, nil
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
