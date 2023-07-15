package backend

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
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
		Bills      []*sdk.Bill            `json:"bills"`
		DCMetadata map[string]*DCMetadata `json:"dcMetadata,omitempty"`
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
	apiV1.HandleFunc("/locked-fee-credit/{systemId}/{billId}", api.getLockedFeeCreditFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/closed-fee-credit/{billId}", api.getClosedFeeCreditFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/transactions/{pubkey}", api.postTransactions).Methods("POST", "OPTIONS")

	apiV1.Handle("/swagger/swagger-initializer.js", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initializer := "swagger/swagger-initializer-money.js"
		f, err := sdk.SwaggerFiles.ReadFile(initializer)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "failed to read %v file: %v", initializer, err)
			return
		}
		http.ServeContent(w, r, "swagger-initializer.js", time.Time{}, bytes.NewReader(f))
	})).Methods("GET", "OPTIONS")
	apiV1.Handle("/swagger/{.*}", http.StripPrefix("/api/v1/", http.FileServer(http.FS(sdk.SwaggerFiles)))).Methods("GET", "OPTIONS")
	apiV1.Handle("/swagger/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := sdk.SwaggerFiles.ReadFile("swagger/index.html")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "failed to read swagger/index.html file: %v", err)
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(f))
	})).Methods("GET", "OPTIONS")

	return router
}

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
	var filteredBills []*sdk.Bill
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
		filteredBills = append(filteredBills, b.ToGenericBill())
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

	api.rw.WriteResponse(w, &ListBillsResponse{
		Bills: filteredBills[offset : offset+limit],
		Total: len(filteredBills),
		DCMetadata: dcMetadataMap,
	})
}

func (api *moneyRestAPI) txHistoryFunc(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	senderPubkey, err := sdk.DecodePubKeyHex(vars["pubkey"])
	if err != nil {
		api.rw.InvalidParamResponse(w, "pubkey", fmt.Errorf("failed to parse sender pubkey: %w", err))
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
	recs, nextKey, err := api.Service.GetTxHistoryRecords(senderPubkey.Hash(), startKey, limit)
	if err != nil {
		log.Error("error on GET /tx-history: ", err)
		api.rw.WriteErrorResponse(w, fmt.Errorf("unable to fetch tx history records: %w", err))
		return
	}
	// check if unconfirmed tx-s are now confirmed or failed
	var roundNr uint64 = 0
	for _, rec := range recs {
		// TODO: update db if stage changes to confirmed or failed
		if rec.State == sdk.UNCONFIRMED {
			proof, err := api.Service.GetTxProof(rec.UnitID, rec.TxHash)
			if err != nil {
				api.rw.WriteErrorResponse(w, fmt.Errorf("failed to fetch tx proof: %w", err))
			}
			if proof != nil {
				rec.State = sdk.CONFIRMED
			} else {
				if roundNr == 0 {
					roundNr, err = api.Service.GetRoundNumber(r.Context())
					if err != nil {
						api.rw.WriteErrorResponse(w, fmt.Errorf("unable to fetch latest round number: %w", err))
					}
				}
				if roundNr > rec.Timeout {
					rec.State = sdk.FAILED
				}
			}
		}
	}
	sdk.SetLinkHeader(r.URL, w, sdk.EncodeHex(nextKey))
	api.rw.WriteCborResponse(w, recs)
}

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

func (api *moneyRestAPI) blockHeightFunc(w http.ResponseWriter, r *http.Request) {
	lastRoundNumber, err := api.Service.GetRoundNumber(r.Context())
	if err != nil {
		log.Error("GET /round-number error fetching round number", err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		api.rw.WriteResponse(w, &RoundNumberResponse{RoundNumber: lastRoundNumber})
	}
}

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

// @Summary Returns last seen transferFC transaction for given partition and fee credit bill
// @Id 7
// @version 1.0
// @produce application/cbor
// @Param systemId path string true "Target Fee Credit System ID of the TransferFC transaction (hex)"
// @Param billId path string true "Target fee credit bill ID (hex)"
// @Success 200 {object} wallet
// @Router /locked-fee-credit/{systemId}/{billId} [get]
func (api *moneyRestAPI) getLockedFeeCreditFunc(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	systemID, err := sdk.ParseHex[sdk.UnitID](vars["systemId"], true)
	if err != nil {
		log.Debug("error parsing GET /locked-fee-credit request systemId param: ", err)
		api.rw.InvalidParamResponse(w, "systemId", err)
		return
	}
	fcbID, err := sdk.ParseHex[sdk.UnitID](vars["billId"], true)
	if err != nil {
		log.Debug("error parsing GET /fee-credit-bills request billId param: ", err)
		api.rw.InvalidParamResponse(w, "billId", err)
		return
	}
	lfc, err := api.Service.GetLockedFeeCredit(systemID, fcbID)
	if err != nil {
		log.Error("error on GET /locked-fee-credit: ", err)
		api.rw.WriteErrorResponse(w, err)
		return
	}
	if lfc == nil {
		api.rw.ErrorResponse(w, http.StatusNotFound, errors.New("locked fee credit does not exist"))
		return
	}
	api.rw.WriteCborResponse(w, lfc)
}

// @Summary Returns last seen closeFC transaction for given fee credit bill
// @Id 8
// @version 1.0
// @produce application/json
// @Param billId path string true "Target fee credit bill ID (hex)"
// @Success 200 {object} wallet
// @Router /closed-fee-credit/{billId} [get]
func (api *moneyRestAPI) getClosedFeeCreditFunc(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fcbID, err := sdk.ParseHex[sdk.UnitID](vars["billId"], true)
	if err != nil {
		log.Debug("error parsing GET /closed-fee-credit request billId param: ", err)
		api.rw.InvalidParamResponse(w, "billId", err)
		return
	}
	cfc, err := api.Service.GetClosedFeeCredit(fcbID)
	if err != nil {
		log.Error("error on GET /closed-fee-credit: ", err)
		api.rw.WriteErrorResponse(w, err)
		return
	}
	if cfc == nil {
		api.rw.ErrorResponse(w, http.StatusNotFound, errors.New("closed fee credit does not exist"))
		return
	}
	api.rw.WriteCborResponse(w, cfc)
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
