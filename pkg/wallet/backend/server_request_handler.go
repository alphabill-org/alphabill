package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/alphabill-org/alphabill/internal/block"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
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
		BillId      string            `json:"billId"`
		BlockNumber uint64            `json:"blockNumber"`
		BlockProof  *block.BlockProof `json:"blockProof"`
	}

	AddKeyRequest struct {
		Pubkey string `json:"pubkey"`
	}

	AddKeyResponse struct {
	}

	ErrorResponse struct {
		Message string `json:"message"`
	}

	BillVM struct {
		Id    string `json:"id"`
		Value uint64 `json:"value"`
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
		wlog.Debug("error parsing GET /list-bills request ", err)
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
		wlog.Error("error on GET /list-bills ", err)
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
	pk, err := parsePubKey(r)
	if err != nil {
		wlog.Debug("error parsing GET /balance request ", err)
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
		wlog.Debug("error parsing GET /block-proof request ", err)
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
		wlog.Error("error on GET /block-proof ", err)
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
		w.WriteHeader(http.StatusInternalServerError)
	}
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
	bytes32 := p.BillId.Bytes32()
	return &BlockProofResponse{
		BillId:      hexutil.Encode(bytes32[:]),
		BlockNumber: p.BlockNumber,
		BlockProof:  p.BlockProof,
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
		bytes32 := b.Id.Bytes32()
		billVMs[i] = &BillVM{
			Id:    hexutil.Encode(bytes32[:]),
			Value: b.Value,
		}
	}
	return billVMs
}
