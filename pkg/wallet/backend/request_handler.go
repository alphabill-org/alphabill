package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"

	"github.com/alphabill-org/alphabill/internal/block"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	txverifier "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_verifier"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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
		Total int           `json:"total"`
		Bills []*ListBillVM `json:"bills"`
	}

	ListBillVM struct {
		Id     []byte `json:"id"`
		Value  uint64 `json:"value"`
		TxHash []byte `json:"txHash"`
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance"`
	}

	AddKeyRequest struct {
		Pubkey string `json:"pubkey"`
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
		if errors.Is(err, ErrPubKeyNotIndexed) {
			wlog.Debug("error on GET /list-bills: ", err)
			w.WriteHeader(http.StatusBadRequest)
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			wlog.Error("error on GET /list-bills: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
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
		if errors.Is(err, ErrPubKeyNotIndexed) {
			wlog.Debug("error on GET /balance: ", err)
			w.WriteHeader(http.StatusBadRequest)
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			wlog.Error("error on GET /balance: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
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
	bill, err := s.service.GetBill(billId)
	if err != nil {
		if errors.Is(err, ErrMissingBlockProof) {
			wlog.Debug("error on GET /block-proof: ", err)
			w.WriteHeader(http.StatusBadRequest)
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			wlog.Error("error on GET /block-proof: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	if bill == nil {
		wlog.Debug("GET /block-proof does not exist ", fmt.Sprintf("%X\n", billId))
		w.WriteHeader(400)
		writeAsJson(w, ErrorResponse{Message: "block proof does not exist for given bill id"})
		return
	}
	writeAsProtoJson(w, bill.toProtoBills())
}

func (s *RequestHandler) setBlockProofFunc(w http.ResponseWriter, r *http.Request) {
	pubkey, err := parsePubKeyQueryParam(r)
	if err != nil {
		wlog.Debug("error parsing POST /block-proof request: ", err)
		w.WriteHeader(http.StatusBadRequest)
		if errors.Is(err, errMissingPubKeyQueryParam) || errors.Is(err, errInvalidPubKeyLength) {
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			writeAsJson(w, ErrorResponse{Message: "invalid pubkey format"})
		}
		return
	}
	req, err := s.readBillsProto(r)
	if err != nil {
		wlog.Debug("error decoding POST /block-proof request: ", err)
		w.WriteHeader(http.StatusBadRequest)
		writeAsJson(w, ErrorResponse{Message: "invalid request body"})
		return
	}
	bills := newBillsFromProto(req.Bills)
	err = s.service.SetBills(pubkey, bills...)
	if err != nil {
		if errors.Is(err, errEmptyBillsList) ||
			errors.Is(err, errKeyNotIndexed) ||
			errors.Is(err, block.ErrProofVerificationFailed) ||
			errors.Is(err, txverifier.ErrVerificationFailed) {
			wlog.Debug("verification error POST /block-proof request: ", err)
			w.WriteHeader(http.StatusBadRequest)
			writeAsJson(w, ErrorResponse{Message: err.Error()})
		} else {
			wlog.Error("error on POST /block-proof: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	writeAsJson(w, EmptyResponse{})
}

func (s *RequestHandler) readBillsProto(r *http.Request) (*block.Bills, error) {
	b, err := ioutil.ReadAll(r.Body)
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

func writeAsProtoJson(w http.ResponseWriter, res proto.Message) {
	w.Header().Set("Content-Type", "application/json")
	bytes, err := protojson.Marshal(res)
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
			Id:     b.Id,
			Value:  b.Value,
			TxHash: b.TxHash,
		}
	}
	return billVMs
}
