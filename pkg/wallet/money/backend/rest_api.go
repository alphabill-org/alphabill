package backend

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
)

const (
	paramIncludeDcBills = "includeDcBills"
	paramPubKey         = "pubkey"
)

type (
	moneyRestAPI struct {
		Service            WalletBackendService
		ListBillsPageLimit int
		rw                 *sdk.ResponseWriter
		log                *slog.Logger
		SystemID           []byte
	}

	ListBillsResponse struct {
		Bills []*sdk.Bill `json:"bills"`
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
	errInvalidBillIDLength = errors.New("bill_id hex string must be 68 characters long (with 0x prefix)")
)

func (api *moneyRestAPI) Router() *mux.Router {
	// TODO add request/response headers middleware
	router := mux.NewRouter().StrictSlash(true)

	apiRouter := router.PathPrefix("/api").Subrouter()
	// add cors middleware
	// content-type needs to be explicitly defined without this content-type header is not allowed and cors filter is not applied
	// Link header is needed for pagination support.
	// OPTIONS method needs to be explicitly defined for each handler func
	apiRouter.Use(handlers.CORS(
		handlers.AllowedHeaders([]string{sdk.ContentType}),
		handlers.ExposedHeaders([]string{sdk.HeaderLink}),
	))

	// version v1 router
	apiV1 := apiRouter.PathPrefix("/v1").Subrouter()
	apiV1.HandleFunc("/list-bills", api.listBillsFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/balance", api.balanceFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/tx-history/{pubkey}", api.txHistoryFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/units/{unitId}/transactions/{txHash}/proof", api.getTxProof).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/round-number", api.blockHeightFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/fee-credit-bills/{billId}", api.getFeeCreditBillFunc).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/transactions/{pubkey}", api.postTransactions).Methods("POST", "OPTIONS")
	apiV1.HandleFunc("/info", api.getInfo).Methods("GET", "OPTIONS")

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
		api.rw.InvalidParamResponse(w, paramPubKey, err)
		return
	}
	includeDCBills, err := parseIncludeDCBillsQueryParam(r, true)
	if err != nil {
		api.rw.InvalidParamResponse(w, paramIncludeDcBills, err)
		return
	}
	qp := r.URL.Query()
	offsetKey, err := sdk.ParseHex[types.UnitID](qp.Get(sdk.QueryParamOffsetKey), false)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamOffsetKey, err)
		return
	}
	limit, err := sdk.ParseMaxResponseItems(qp.Get(sdk.QueryParamLimit), api.ListBillsPageLimit)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamLimit, err)
		return
	}
	bills, nextKey, err := api.Service.GetBills(pk, includeDCBills, offsetKey, limit)
	if err != nil {
		api.rw.WriteErrorResponse(w, err)
		return
	}

	// convert to sdk struct
	var sdkBills []*sdk.Bill
	for _, b := range bills {
		sdkBills = append(sdkBills, b.ToGenericBill())
	}

	sdk.SetLinkHeader(r.URL, w, sdk.EncodeHex(nextKey))
	api.rw.WriteResponse(w, &ListBillsResponse{
		Bills: sdkBills,
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
		api.log.LogAttrs(r.Context(), slog.LevelError, "error on GET /tx-history", logger.Error(err))
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
		api.rw.InvalidParamResponse(w, paramPubKey, err)
		return
	}
	includeDCBills, err := parseIncludeDCBillsQueryParam(r, false)
	if err != nil {
		api.rw.InvalidParamResponse(w, paramIncludeDcBills, err)
		return
	}
	balance, err := api.getBalance(pk, includeDCBills)
	if err != nil {
		api.rw.WriteErrorResponse(w, err)
		return
	}
	res := &BalanceResponse{Balance: balance}
	api.rw.WriteResponse(w, res)
}

func (api *moneyRestAPI) getBalance(pubKey []byte, includeDCBills bool) (uint64, error) {
	var balance uint64
	var offsetKey []byte
	for {
		bills, nextKey, err := api.Service.GetBills(pubKey, includeDCBills, offsetKey, api.ListBillsPageLimit)
		if err != nil {
			return 0, err
		}
		for _, b := range bills {
			balance += b.Value
		}
		if nextKey == nil {
			break
		}
		offsetKey = nextKey
	}
	return balance, nil
}

func (api *moneyRestAPI) getTxProof(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitID, err := sdk.ParseHex[types.UnitID](vars["unitId"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "unitId", err)
		return
	}
	if len(unitID) != money.UnitIDLength {
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
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to load proof of tx 0x%X (unit 0x%s): %w", txHash, unitID, err))
		return
	}
	if proof == nil {
		api.rw.ErrorResponse(w, http.StatusNotFound, fmt.Errorf("no proof found for tx 0x%X (unit 0x%s)", txHash, unitID))
		return
	}

	api.rw.WriteCborResponse(w, proof)
}

func (api *moneyRestAPI) blockHeightFunc(w http.ResponseWriter, r *http.Request) {
	lastRoundNumber, err := api.Service.GetRoundNumber(r.Context())
	if err != nil {
		api.log.LogAttrs(r.Context(), slog.LevelError, "GET /round-number error fetching round number", logger.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		api.rw.WriteResponse(w, &RoundNumberResponse{RoundNumber: lastRoundNumber})
	}
}

func (api *moneyRestAPI) getFeeCreditBillFunc(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	billID, err := sdk.ParseHex[types.UnitID](vars["billId"], true)
	if err != nil {
		api.log.LogAttrs(r.Context(), slog.LevelDebug, "error parsing GET /fee-credit-bills request", logger.Error(err))
		if errors.Is(err, errInvalidBillIDLength) {
			api.rw.ErrorResponse(w, http.StatusBadRequest, err)
		} else {
			api.rw.InvalidParamResponse(w, "billId", err)
		}
		return
	}
	if len(billID) != 33 {
		api.rw.ErrorResponse(w, http.StatusBadRequest, errInvalidBillIDLength)
		return
	}
	fcb, err := api.Service.GetFeeCreditBill(billID)
	if err != nil {
		api.log.LogAttrs(r.Context(), slog.LevelError, "error on GET /fee-credit-bill", logger.Error(err))
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
		api.log.LogAttrs(r.Context(), slog.LevelDebug, "error parsing GET /transactions request", logger.Error(err))
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
		for k, v := range errs {
			err = fmt.Errorf("%s: %s", k, v)
			api.log.LogAttrs(r.Context(), slog.LevelDebug, "error on POST /transactions", logger.Error(err))
		}
		w.WriteHeader(http.StatusInternalServerError)
		api.rw.WriteResponse(w, errs)
		return
	}

	if err = egp.Wait(); err != nil {
		api.log.LogAttrs(r.Context(), slog.LevelDebug, "failed to store tx metadata", logger.Error(err))
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to store tx metadata: %w", err))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (api *moneyRestAPI) getInfo(w http.ResponseWriter, _ *http.Request) {
	systemID := hex.EncodeToString(api.SystemID)
	res := sdk.InfoResponse{
		SystemID: systemID,
		Name:     "money backend",
	}
	api.rw.WriteResponse(w, res)
}

func parsePubKeyQueryParam(r *http.Request) (sdk.PubKey, error) {
	return sdk.DecodePubKeyHex(r.URL.Query().Get(paramPubKey))
}

func parseIncludeDCBillsQueryParam(r *http.Request, defaultValue bool) (bool, error) {
	if r.URL.Query().Has(paramIncludeDcBills) {
		return strconv.ParseBool(r.URL.Query().Get(paramIncludeDcBills))
	}
	return defaultValue, nil
}
