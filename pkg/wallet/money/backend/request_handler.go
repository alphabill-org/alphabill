package backend

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	_ "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/docs"
)

type (
	moneyRestAPI struct {
		Service            WalletBackendService
		ListBillsPageLimit int
		rw                 *sdk.ResponseWriter
	}

	// TODO: perhaps pass the total number of elements in a response header
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
)

var (
	errMissingPubKeyQueryParam = errors.New("missing required pubkey query parameter")
	errInvalidPubKeyLength     = errors.New("pubkey hex string must be 68 characters long (with 0x prefix)")
	errMissingBillIDQueryParam = errors.New("missing required bill_id query parameter")
	errInvalidBillIDLength     = errors.New("bill_id hex string must be 66 characters long (with 0x prefix)")
)

func (api *moneyRestAPI) Router() *mux.Router {
	// TODO add request/response headers middleware
	router := mux.NewRouter().StrictSlash(true)

	apiRouter := router.PathPrefix("/api").Subrouter()
	// add cors middleware
	// content-type needs to be explicitly defined without this content-type header is not allowed and cors filter is not applied
	// OPTIONS method needs to be explicitly defined for each handler func
	apiRouter.Use(handlers.CORS(handlers.AllowedHeaders([]string{sdk.ContentType})))

	// version v1 router
	apiV1 := apiRouter.PathPrefix("/v1").Subrouter()
	apiV1.HandleFunc("/list-bills", api.listBillsFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/balance", api.balanceFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/units/{unitId}/transactions/{txHash}/proof", api.getTxProof).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/round-number", api.blockHeightFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/fee-credit-bills/{billId}", api.getFeeCreditBillFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/transactions/{pubkey}", api.postTransactions).Methods("POST", "OPTIONS")

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
// @Failure 400 {object} ErrorResponse
// @Failure 500
// @Router /list-bills [get]
func (api *moneyRestAPI) listBillsFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		log.Debug("error parsing GET /list-bills request: ", err)
		api.rw.InvalidParamResponse(w, "pubkey", err)
		return
	}
	includeDCBills, err := parseIncludeDCBillsQueryParam(r, true)
	if err != nil {
		log.Debug("error parsing GET /balance request: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	bills, err := api.Service.GetBills(pk)
	if err != nil {
		log.Error("error on GET /list-bills: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var filteredBills []*Bill
	for _, b := range bills {
		// filter dc bills
		if b.IsDCBill && !includeDCBills {
			continue
		}
		// filter zero value bills
		if b.Value == 0 {
			continue
		}
		filteredBills = append(filteredBills, b)
	}
	limit, offset := parsePagingParams(api.ListBillsPageLimit, r)
	// if offset and limit go out of bounds just return what we have
	if offset > len(filteredBills) {
		offset = len(filteredBills)
	}
	if offset+limit > len(filteredBills) {
		limit = len(filteredBills) - offset
	} else {
		setLinkHeader(r.URL, w, offset+limit)
	}
	res := newListBillsResponse(filteredBills, limit, offset)
	api.rw.WriteResponse(w, res)
}

// @Summary Get balance
// @Id 2
// @version 1.0
// @produce application/json
// @Param pubkey query string true "Public key prefixed with 0x"
// @Success 200 {object} BalanceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500
// @Router /balance [get]
func (api *moneyRestAPI) balanceFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		log.Debug("error parsing GET /balance request: ", err)
		api.rw.InvalidParamResponse(w, "pubkey", err)
		return
	}
	includeDCBills, err := parseIncludeDCBillsQueryParam(r, false)
	if err != nil {
		log.Debug("error parsing GET /balance request: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	bills, err := api.Service.GetBills(pk)
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
	api.rw.WriteResponse(w, res)
}

func (api *moneyRestAPI) getTxProof(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitID, err := sdk.ParseHex[sdk.UnitID](vars["unitId"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "unitId", err)
		return
	}
	if len(unitID) != 32 {
		api.rw.ErrorResponse(w, http.StatusBadRequest, errInvalidBillIDLength)
		return
	}
	txHash, err := sdk.ParseHex[sdk.TxHash](vars["txHash"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "txHash", err)
		return
	}

	bill, err := api.getBill(unitID)
	if err != nil {
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to load bill (id 0x%X): %w", unitID, err))
		return
	}
	if bill == nil {
		log.Debug("error on GET /proof: ", err)
		api.rw.ErrorResponse(w, http.StatusNotFound, fmt.Errorf("bill (id 0x%X) does not exist", unitID))
		return
	}
	log.Info(fmt.Sprintf("getTxProof: bill.TxHash: %x, txHash: %x", bill.TxHash, txHash))
	if !bytes.Equal(bill.TxHash, txHash) {
		api.rw.ErrorResponse(w, http.StatusNotFound, fmt.Errorf("no proof found for tx 0x%X (unit 0x%X)", txHash, unitID))
		return
	}
	api.rw.WriteCborResponse(w, bill.TxProof)
}

// getBill returns "normal" or "fee credit" bill for given id,
// this is necessary to facilitate client side tx confirmation,
// alternatively we could refactor how tx proofs are stored (separately from bills),
// in that case we could fetch proofs directly and this hack would not be necessary
func (api *moneyRestAPI) getBill(billID []byte) (*Bill, error) {
	bill, err := api.Service.GetBill(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bill: %w", err)
	}
	if bill == nil {
		bill, err = api.Service.GetFeeCreditBill(billID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
		}
	}
	return bill, err
}

// @Summary Money partition's latest block number
// @Id 4
// @version 1.0
// @produce application/json
// @Success 200 {object} RoundNumberResponse
// @Router /round-number [get]
func (api *moneyRestAPI) blockHeightFunc(w http.ResponseWriter, r *http.Request) {
	lastRoundNumber, err := api.Service.GetRoundNumber(r.Context())
	if err != nil {
		log.Error("GET /round-number error fetching round number", err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		api.rw.WriteResponse(w, &RoundNumberResponse{RoundNumber: lastRoundNumber})
	}
}

// @Summary Get Fee Credit Bill
// @Id 5
// @version 1.0
// @produce application/json
// @Param billId path string true "ID of the bill (hex)"
// @Success 200 {object} wallet.Bill
// @Router /fee-credit-bills [get]
func (api *moneyRestAPI) getFeeCreditBillFunc(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	billID, err := sdk.ParseHex[sdk.UnitID](vars["billId"], true)
	if err != nil {
		log.Debug("error parsing GET /fee-credit-bills request: ", err)
		if errors.Is(err, errInvalidBillIDLength) {
			api.rw.ErrorResponse(w, http.StatusBadRequest, err)
		} else {
			api.rw.InvalidParamResponse(w, "billId", err)
		}
		return
	}
	if len(billID) != 32 {
		api.rw.ErrorResponse(w, http.StatusBadRequest, errInvalidBillIDLength)
		return
	}
	fcb, err := api.Service.GetFeeCreditBill(billID)
	if err != nil {
		log.Error("error on GET /fee-credit-bill: ", err)
		api.rw.WriteErrorResponse(w, err)
		return
	}
	if fcb == nil {
		api.rw.ErrorResponse(w, http.StatusNotFound, errors.New("fee credit bill does not exist"))
		return
	}
	api.rw.WriteResponse(w, fcb.ToGenericBill())
}

// @Summary Forward transactions to partition node(s)
// @Id 6
// @Version 1.0
// @Accept application/cbor
// @Param pubkey path string true "Sender public key prefixed with 0x"
// @Param transactions body nil true "CBOR encoded array of TransactionOrders"
// @Success 202
// @Router /transactions [post]
func (api *moneyRestAPI) postTransactions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to read request body: %w", err))
		return
	}

	vars := mux.Vars(r)
	_, err = sdk.DecodePubKeyHex(vars["pubkey"])
	if err != nil {
		api.rw.InvalidParamResponse(w, "pubkey", fmt.Errorf("failed to parse sender pubkey: %w", err))
		return
	}

	txs := &sdk.Transactions{}
	if err = cbor.Unmarshal(buf, txs); err != nil {
		api.rw.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("failed to decode request body: %w", err))
		return
	}
	if len(txs.Transactions) == 0 {
		api.rw.ErrorResponse(w, http.StatusBadRequest, errors.New("request body contained no transactions to process"))
		return
	}

	if errs := api.Service.SendTransactions(r.Context(), txs.Transactions); len(errs) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		api.rw.WriteResponse(w, errs)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func parsePubKeyQueryParam(r *http.Request) ([]byte, error) {
	return sdk.DecodePubKeyHex(r.URL.Query().Get("pubkey"))
}

func parseIncludeDCBillsQueryParam(r *http.Request, defaultValue bool) (bool, error) {
	if r.URL.Query().Has("includedcbills") {
		return strconv.ParseBool(r.URL.Query().Get("includedcbills"))
	}
	return defaultValue, nil
}

func parsePagingParams(pageLimit int, r *http.Request) (int, int) {
	limit := parseInt(r.URL.Query().Get("limit"), pageLimit)
	if limit < 0 {
		limit = 0
	}
	if limit > pageLimit {
		limit = pageLimit
	}
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func setLinkHeader(u *url.URL, w http.ResponseWriter, offset int) {
	if offset < 0 {
		w.Header().Del("Link")
		return
	}
	qp := u.Query()
	qp.Set("offset", strconv.Itoa(offset))
	u.RawQuery = qp.Encode()
	w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, u))
}

func parseInt(str string, def int) int {
	num, err := strconv.Atoi(str)
	if err != nil {
		return def
	}
	return num
}

func newListBillsResponse(bills []*Bill, limit, offset int) *ListBillsResponse {
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

func (b *ListBillVM) ToGenericBill() *sdk.Bill {
	return &sdk.Bill{
		Id:       b.Id,
		Value:    b.Value,
		TxHash:   b.TxHash,
		IsDcBill: b.IsDCBill,
	}
}
