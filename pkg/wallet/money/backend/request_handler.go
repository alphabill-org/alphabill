package backend

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/sync/errgroup"

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
		Total      int                    `json:"total" example:"1"`
		Bills      []*ListBillVM          `json:"bills"`
		DCMetadata map[string]*DCMetadata `json:"dcMetadata,omitempty"`
	}

	ListBillVM struct {
		Id      []byte `json:"id" swaggertype:"string" format:"base64" example:"AAAAAAgwv3UA1HfGO4qc1T3I3EOvqxfcrhMjJpr9Tn4="`
		Value   uint64 `json:"value,string" example:"1000"`
		TxHash  []byte `json:"txHash" swaggertype:"string" format:"base64" example:"Q4ShCITC0ODXPR+j1Zl/teYcoU3/mAPy0x8uSsvQFM8="`
		DcNonce []byte `json:"dcNonce" swaggertype:"string" format:"base64" example:"YWZhIHNmc2RmYXNkZmFzIGRmc2FzZiBhc2RmIGFzZGZzYSBkZg=="`
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
	errInvalidBillIDLength = errors.New("bill_id hex string must be 66 characters long (with 0x prefix)")
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
	apiV1.HandleFunc("/tx-history/{pubkey}", api.txHistoryFunc).Methods("GET", "OPTIONS")
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
// @Param includedcbills query bool false "response will include DC bills" default(true)
// @Param includedcmetadata query bool false "response will include DC Metadata info (includedcbills param must also be true)" default(false)
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
		log.Debug("error parsing GET /list-bills request: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var includeDCMetadata bool
	if r.URL.Query().Has("includedcmetadata") {
		includeDCMetadata, err = strconv.ParseBool(r.URL.Query().Get("includedcmetadata"))
		if err != nil {
			log.Debug("error parsing GET /list-bills request: ", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	bills, err := api.Service.GetBills(pk)
	if err != nil {
		log.Error("error on GET /list-bills: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var filteredBills []*Bill
	var dcMetadataMap map[string]*DCMetadata
	for _, b := range bills {
		// filter zero value bills
		if b.Value == 0 {
			continue
		}
		// filter dc bills
		if b.DcNonce != nil {
			if !includeDCBills {
				continue
			}
			if includeDCMetadata {
				if dcMetadataMap == nil {
					dcMetadataMap = make(map[string]*DCMetadata)
				}
				if dcMetadataMap[string(b.DcNonce)] == nil {
					dcMetadata, err := api.Service.GetDCMetadata(b.DcNonce)
					if err != nil {
						log.Error("error on GET /list-bills: ", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					dcMetadataMap[string(b.DcNonce)] = dcMetadata
				}
			}
		}
		filteredBills = append(filteredBills, b)
	}
	qp := r.URL.Query()
	limit, err := sdk.ParseMaxResponseItems(qp.Get(sdk.QueryParamLimit), api.ListBillsPageLimit)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamLimit, err)
		return
	}
	offset := sdk.ParseIntParam(qp.Get(sdk.QueryParamOffsetKey), 0)
	if offset < 0 {
		offset = 0
	}
	// if offset and limit go out of bounds just return what we have
	if offset > len(filteredBills) {
		offset = len(filteredBills)
	}
	if offset+limit > len(filteredBills) {
		limit = len(filteredBills) - offset
	} else {
		sdk.SetLinkHeader(r.URL, w, strconv.Itoa(offset+limit))
	}
	billVMs := toBillVMList(filteredBills)
	api.rw.WriteResponse(w, &ListBillsResponse{Bills: billVMs[offset : offset+limit], Total: len(billVMs), DCMetadata: dcMetadataMap})
}

func (api *moneyRestAPI) txHistoryFunc(w http.ResponseWriter, r *http.Request) {
	pk, err := parsePubKeyQueryParam(r)
	if err != nil {
		log.Debug("error parsing GET /balance request: ", err)
		api.rw.InvalidParamResponse(w, "pubkey", err)
		return
	}
	qp := r.URL.Query()
	startKey, err := sdk.ParseHex[[]byte](qp.Get(sdk.QueryParamOffsetKey), false)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamOffsetKey, err)
		return
	}

	limit, err := sdk.ParseMaxResponseItems(qp.Get(sdk.QueryParamLimit), api.ListBillsPageLimit)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamLimit, err)
		return
	}
	recs, nextKey, err := api.Service.GetTxHistoryRecords(pk.Hash(), startKey, limit)
	if err != nil {
		log.Error("error on GET /tx-history: ", err)
		api.rw.WriteErrorResponse(w, fmt.Errorf("error on GET /tx-history: %w", err))
		return
	}

	sdk.SetLinkHeader(r.URL, w, sdk.EncodeHex(nextKey))
	api.rw.WriteCborResponse(w, recs)
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
		if b.DcNonce == nil || includeDCBills {
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

	proof, err := api.Service.GetTxProof(unitID, txHash)
	if err != nil {
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to load proof of tx 0x%X (unit 0x%X): %w", txHash, unitID, err))
		return
	}
	if proof == nil {
		api.rw.ErrorResponse(w, http.StatusNotFound, fmt.Errorf("no proof found for tx 0x%X (unit 0x%X)", txHash, unitID))
		return
	}

	api.rw.WriteCborResponse(w, proof)
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
	senderPubkey, err := sdk.DecodePubKeyHex(vars["pubkey"])
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

	egp, _ := errgroup.WithContext(r.Context())
	api.Service.HandleTransactionsSubmission(egp, senderPubkey, txs.Transactions)

	if errs := api.Service.SendTransactions(r.Context(), txs.Transactions); len(errs) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		api.rw.WriteResponse(w, errs)
		return
	}

	if err = egp.Wait(); err != nil {
		log.Debug("failed to store DC metadata: ", err)
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to store DC metadata: %w", err))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func parsePubKeyQueryParam(r *http.Request) (sdk.PubKey, error) {
	return sdk.DecodePubKeyHex(r.URL.Query().Get("pubkey"))
}

func parseIncludeDCBillsQueryParam(r *http.Request, defaultValue bool) (bool, error) {
	if r.URL.Query().Has("includedcbills") {
		return strconv.ParseBool(r.URL.Query().Get("includedcbills"))
	}
	return defaultValue, nil
}

func toBillVMList(bills []*Bill) []*ListBillVM {
	billVMs := make([]*ListBillVM, len(bills))
	for i, b := range bills {
		billVMs[i] = &ListBillVM{
			Id:      b.Id,
			Value:   b.Value,
			TxHash:  b.TxHash,
			DcNonce: b.DcNonce,
		}
	}
	return billVMs
}

func (b *ListBillVM) ToGenericBill() *sdk.Bill {
	return &sdk.Bill{
		Id:      b.Id,
		Value:   b.Value,
		TxHash:  b.TxHash,
		DcNonce: b.DcNonce,
	}
}
