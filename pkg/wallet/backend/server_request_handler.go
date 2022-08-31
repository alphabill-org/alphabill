package backend

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
)

type (
	RequestHandler struct {
		service WalletBackendService
	}

	ListBillsResponse struct {
		Bills []*bill `json:"bills"`
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance"`
	}

	BlockProofResponse struct {
		BlockProof *blockProof `json:"blockProof"`
	}

	ErrorResponse struct {
		Message string `json:"message"`
	}
)

func (s *RequestHandler) router() *mux.Router {
	// TODO add request/response headers middleware
	r := mux.NewRouter().StrictSlash(true)
	r.HandleFunc("/list-bills", s.listBillsFunc).Methods("GET")
	r.HandleFunc("/balance", s.balanceFunc).Methods("GET")
	r.HandleFunc("/block-proof", s.blockProofFunc).Methods("GET")
	return r
}

func (s *RequestHandler) listBillsFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKey(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		return
	}
	bills := s.service.GetBills(pk)
	res := &ListBillsResponse{Bills: bills}
	writeAsJson(w, res)
}

func (s *RequestHandler) balanceFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKey(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: err.Error()})
		return
	}
	bills := s.service.GetBills(pk)
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
		return
	}
	p := s.service.GetBlockProof(billId)
	res := &BlockProofResponse{p}
	writeAsJson(w, res)
}

func writeAsJson(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func parseBillId(r *http.Request) ([]byte, error) {
	billId := r.URL.Query().Get("bill_id")
	if billId == "" {
		return nil, errors.New("missing required bill_id query parameter")
	}
	return hexutil.Decode(billId)
}

func parsePubKey(r *http.Request) ([]byte, error) {
	pubKey := r.URL.Query().Get("pubkey")
	if pubKey == "" {
		return nil, errors.New("missing required pubkey query parameter")
	}
	return decodePubKeyHex(pubKey)
}

func decodePubKeyHex(pubKey string) ([]byte, error) {
	if len(pubKey) != 68 {
		return nil, errors.New("pubkey must be 68 bytes long (with 0x prefix)")
	}
	return hexutil.Decode(pubKey)
}
