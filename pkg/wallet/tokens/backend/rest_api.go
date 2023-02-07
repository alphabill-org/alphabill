package twb

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
)

type dataSource interface {
	GetBlockNumber() (uint64, error)
	QueryTokenType(kind Kind, creator PubKey, startKey []byte, count int) ([]*TokenUnitType, []byte, error)
	QueryTokens(kind Kind, owner Predicate, startKey []byte, count int) ([]*TokenUnit, []byte, error)
	SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator PubKey) error
}

type restAPI struct {
	db              dataSource
	sendTransaction func(*txsystem.Transaction) (*txsystem.TransactionResponse, error)
	convertTx       func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)
	logErr          func(a ...any)
}

func (api *restAPI) endpoints() http.Handler {
	apiRouter := mux.NewRouter().StrictSlash(true).PathPrefix("/api").Subrouter()

	// add cors middleware
	// content-type needs to be explicitly defined without this content-type header is not allowed and cors filter is not applied
	// OPTIONS method needs to be explicitly defined for each handler func
	apiRouter.Use(handlers.CORS(handlers.AllowedHeaders([]string{"Content-Type"})))

	// version v1 router
	apiV1 := apiRouter.PathPrefix("/v1").Subrouter()
	apiV1.HandleFunc("/kinds/{kind}/owners/{owner}/tokens", api.listTokens).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/kinds/{kind}/types", api.listTypes).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/round-number", api.getRoundNumber).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/transactions/{pubkey}", api.postTransactions).Methods("POST", "OPTIONS")

	return apiRouter
}

func (api *restAPI) listTokens(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner, err := parsePubKey(vars["owner"], true)
	if err != nil {
		api.invalidParamResponse(w, "owner", err)
		return
	}

	kind, err := strToTokenKind(vars["kind"])
	if err != nil {
		api.invalidParamResponse(w, "kind", err)
		return
	}

	qp := r.URL.Query()
	startKey, err := parseTokenID(qp.Get("offsetKey"), false)
	if err != nil {
		api.invalidParamResponse(w, "offsetKey", err)
		return
	}

	data, next, err := api.db.QueryTokens(kind, owner, startKey, maxResponseItems(qp.Get("limit")))
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}
	setLinkHeader(r.URL, w, encodeTokenID(next))
	api.writeResponse(w, data)
}

func (api *restAPI) listTypes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	kind, err := strToTokenKind(vars["kind"])
	if err != nil {
		api.invalidParamResponse(w, "kind", err)
		return
	}

	qp := r.URL.Query()
	creator, err := parsePubKey(qp.Get("creator"), false)
	if err != nil {
		api.invalidParamResponse(w, "creator", err)
		return
	}

	startKey, err := parseTokenTypeID(qp.Get("offsetKey"), false)
	if err != nil {
		api.invalidParamResponse(w, "offsetKey", err)
		return
	}

	data, next, err := api.db.QueryTokenType(kind, creator, startKey, maxResponseItems(qp.Get("limit")))
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}
	setLinkHeader(r.URL, w, encodeTokenTypeID(next))
	api.writeResponse(w, data)
}

func (api *restAPI) getRoundNumber(w http.ResponseWriter, r *http.Request) {
	rn, err := api.db.GetBlockNumber()
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}
	api.writeResponse(w, RoundNumberResponse{RoundNumber: rn})
}

func (api *restAPI) postTransactions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		api.writeErrorResponse(w, fmt.Errorf("failed to read request body: %w", err))
		return
	}

	vars := mux.Vars(r)
	owner, err := parsePubKey(vars["pubkey"], true)
	if err != nil {
		api.invalidParamResponse(w, "pubkey", err)
		return
	}

	txs := &txsystem.Transactions{}
	if err = protojson.Unmarshal(buf, txs); err != nil {
		api.errorResponse(w, http.StatusBadRequest, fmt.Errorf("failed to decode request body: %w", err))
		return
	}
	if len(txs.GetTransactions()) == 0 {
		api.errorResponse(w, http.StatusBadRequest, fmt.Errorf("request body contained no transactions to process"))
		return
	}

	if errs := api.saveTxs(r.Context(), txs.GetTransactions(), owner); len(errs) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		api.writeResponse(w, errs)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (api *restAPI) saveTxs(ctx context.Context, txs []*txsystem.Transaction, owner []byte) map[string]string {
	errs := make(map[string]string)
	var m sync.Mutex

	const maxWorkers = 5
	sem := semaphore.NewWeighted(maxWorkers)
	for _, tx := range txs {
		if err := sem.Acquire(ctx, 1); err != nil {
			break
		}
		go func(tx *txsystem.Transaction) {
			defer sem.Release(1)
			if err := api.saveTx(tx, owner); err != nil {
				m.Lock()
				errs[hex.EncodeToString(tx.GetUnitId())] = err.Error()
				m.Unlock()
			}
		}(tx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sem.Acquire(ctx, maxWorkers); err != nil {
		m.Lock()
		errs["waiting-for-workers"] = err.Error()
		m.Unlock()
	}
	return errs
}

func (api *restAPI) saveTx(tx *txsystem.Transaction, owner []byte) error {
	// if "creator type tx" then save the type->owner relation
	gtx, err := api.convertTx(tx)
	if err != nil {
		return fmt.Errorf("failed to convert transaction: %w", err)
	}
	kind := Any
	switch gtx.(type) {
	case tokens.CreateFungibleTokenType:
		kind = Fungible
	case tokens.CreateNonFungibleTokenType:
		kind = NonFungible
	}
	if kind != Any {
		if err := api.db.SaveTokenTypeCreator(tx.UnitId, kind, owner); err != nil {
			return fmt.Errorf("failed to save creator relation: %w", err)
		}
	}

	rsp, err := api.sendTransaction(tx)
	if err != nil {
		return fmt.Errorf("failed to forward tx: %w", err)
	}
	if !rsp.GetOk() {
		return fmt.Errorf("transaction was not accepted: %s", rsp.GetMessage())
	}
	return nil
}

func (api *restAPI) writeResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		api.logError(fmt.Errorf("failed to encode response data as json: %w", err))
	}
}

func (api *restAPI) writeErrorResponse(w http.ResponseWriter, err error) {
	api.errorResponse(w, http.StatusInternalServerError, err)
	api.logError(err)
}

func (api *restAPI) invalidParamResponse(w http.ResponseWriter, name string, err error) {
	api.errorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid parameter %q: %w", name, err))
}

func (api *restAPI) errorResponse(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Message: err.Error()}); err != nil {
		api.logError(fmt.Errorf("failed to encode error response as json: %w", err))
	}
}

func (api *restAPI) logError(err error) {
	if api.logErr != nil {
		api.logErr(err)
	}
}

func setLinkHeader(u *url.URL, w http.ResponseWriter, next string) {
	if next == "" {
		w.Header().Del("Link")
		return
	}
	qp := u.Query()
	qp.Set("offsetKey", next)
	u.RawQuery = qp.Encode()
	w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, u))
}

func maxResponseItems(s string) int {
	const def = 100 // default / max response item count
	v, err := strconv.Atoi(s)
	if v <= 0 || v > def || err != nil {
		return def
	}
	return v
}

type (
	RoundNumberResponse struct {
		RoundNumber uint64 `json:"roundNumber"`
	}

	ErrorResponse struct {
		Message string `json:"message"`
	}
)
