package backend

import (
	"encoding/json"
	"errors"
	"net/http"

	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/holiman/uint256"
)

type (
	RequestHandler struct {
		service WalletBackendService
	}

	ListBillsResponse struct {
		Bills []*Bill `json:"bills"`
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance"`
	}

	BlockProofResponse struct {
		BlockProof *BlockProof `json:"blockProof"`
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
	bills, err := s.service.GetBills(pk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		wlog.Error("error on GET /list-bills ", err)
		return
	}
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
		return nil, errors.New("missing required bill_id query parameter")
	}
	billId256, err := uint256.FromHex(billIdHex)
	if err != nil {
		return nil, err
	}
	billIdBytes := billId256.Bytes32()
	return billIdBytes[:], nil
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
		return nil, errors.New("pubkey hex string must be 68 characters long (with 0x prefix)")
	}
	return hexutil.Decode(pubKey)
}
