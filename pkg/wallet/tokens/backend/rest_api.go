package backend

import (
	"bytes"
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/sync/semaphore"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/broker"
)

type dataSource interface {
	GetTokenType(id TokenTypeID) (*TokenUnitType, error)
	QueryTokenType(kind Kind, creator wallet.PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error)
	GetToken(id TokenID) (*TokenUnit, error)
	QueryTokens(kind Kind, owner wallet.Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error)
	SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator wallet.PubKey) error
	GetTxProof(unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	GetFeeCreditBill(unitID wallet.UnitID) (*FeeCreditBill, error)
}

type abClient interface {
	SendTransaction(ctx context.Context, tx *types.TransactionOrder) error
	GetRoundNumber(ctx context.Context) (uint64, error)
}

type restAPI struct {
	db        dataSource
	ab        abClient
	streamSSE func(ctx context.Context, owner broker.PubKey, w http.ResponseWriter) error
	logErr    func(a ...any)
}

const maxResponseItems = 100

//go:embed swagger/*
var swaggerFiles embed.FS

func (api *restAPI) endpoints() http.Handler {
	apiRouter := mux.NewRouter().StrictSlash(true).PathPrefix("/api").Subrouter()

	// add cors middleware
	// content-type needs to be explicitly defined without this content-type header is not allowed and cors filter is not applied
	// Link header is needed for pagination support.
	// OPTIONS method needs to be explicitly defined for each handler func
	apiRouter.Use(handlers.CORS(
		handlers.AllowedHeaders([]string{"Content-Type"}),
		handlers.ExposedHeaders([]string{"Link"}),
	))

	// version v1 router
	apiV1 := apiRouter.PathPrefix("/v1").Subrouter()
	apiV1.HandleFunc("/tokens/{tokenId}", api.getToken).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/types/{typeId}/hierarchy", api.typeHierarchy).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/kinds/{kind}/owners/{owner}/tokens", api.listTokens).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/kinds/{kind}/types", api.listTypes).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/round-number", api.getRoundNumber).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/transactions/{pubkey}", api.postTransactions).Methods("POST", "OPTIONS")
	apiV1.HandleFunc("/events/{pubkey}/subscribe", api.subscribeEvents).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/units/{unitId}/transactions/{txHash}/proof", api.getTxProof).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/fee-credit-bills/{unitId}", api.getFeeCreditBill).Methods("GET", "OPTIONS")

	apiV1.Handle("/swagger/{.*}", http.StripPrefix("/api/v1/", http.FileServer(http.FS(swaggerFiles)))).Methods("GET", "OPTIONS")
	apiV1.Handle("/swagger/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := swaggerFiles.ReadFile("swagger/index.html")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "failed to read swagger/index.html file: %v", err)
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(f))
	})).Methods("GET", "OPTIONS")

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
	startKey, err := parseHex[TokenID](qp.Get("offsetKey"), false)
	if err != nil {
		api.invalidParamResponse(w, "offsetKey", err)
		return
	}

	limit, err := parseMaxResponseItems(qp.Get("limit"), maxResponseItems)
	if err != nil {
		api.invalidParamResponse(w, "limit", err)
		return
	}

	data, next, err := api.db.QueryTokens(
		kind,
		script.PredicatePayToPublicKeyHashDefault(hash.Sum256(owner)),
		startKey,
		limit)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}
	setLinkHeader(r.URL, w, encodeHex(next))
	api.writeResponse(w, data)
}

func (api *restAPI) getToken(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tokenId, err := parseHex[TokenID](vars["tokenId"], true)
	if err != nil {
		api.invalidParamResponse(w, "tokenId", err)
		return
	}

	token, err := api.db.GetToken(tokenId)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}
	api.writeResponse(w, token)
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

	startKey, err := parseHex[TokenTypeID](qp.Get("offsetKey"), false)
	if err != nil {
		api.invalidParamResponse(w, "offsetKey", err)
		return
	}

	limit, err := parseMaxResponseItems(qp.Get("limit"), maxResponseItems)
	if err != nil {
		api.invalidParamResponse(w, "limit", err)
		return
	}

	data, next, err := api.db.QueryTokenType(kind, creator, startKey, limit)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}
	setLinkHeader(r.URL, w, encodeHex(next))
	api.writeResponse(w, data)
}

func (api *restAPI) typeHierarchy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	typeId, err := parseHex[TokenTypeID](vars["typeId"], true)
	if err != nil {
		api.invalidParamResponse(w, "typeId", err)
		return
	}

	var rsp []*TokenUnitType
	for len(typeId) > 0 && !bytes.Equal(typeId, NoParent) {
		tokTyp, err := api.db.GetTokenType(typeId)
		if err != nil {
			api.writeErrorResponse(w, fmt.Errorf("failed to load type with id %x: %w", typeId, err))
			return
		}
		rsp = append(rsp, tokTyp)
		typeId = tokTyp.ParentTypeID
	}
	api.writeResponse(w, rsp)
}

func (api *restAPI) getRoundNumber(w http.ResponseWriter, r *http.Request) {
	rn, err := api.ab.GetRoundNumber(r.Context())
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}
	api.writeResponse(w, RoundNumberResponse{RoundNumber: rn})
}

func (api *restAPI) subscribeEvents(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ownerPK, err := parsePubKey(vars["pubkey"], true)
	if err != nil {
		api.invalidParamResponse(w, "pubkey", err)
		return
	}

	if err := api.streamSSE(r.Context(), broker.PubKey(ownerPK), w); err != nil {
		api.writeErrorResponse(w, fmt.Errorf("event streaming failed: %w", err))
	}
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

	txs := &wallet.Transactions{}
	if err = cbor.Unmarshal(buf, txs); err != nil {
		api.errorResponse(w, http.StatusBadRequest, fmt.Errorf("failed to decode request body: %w", err))
		return
	}
	if len(txs.Transactions) == 0 {
		api.errorResponse(w, http.StatusBadRequest, fmt.Errorf("request body contained no transactions to process"))
		return
	}

	if errs := api.saveTxs(r.Context(), txs.Transactions, owner); len(errs) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		api.writeResponse(w, errs)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (api *restAPI) saveTxs(ctx context.Context, txs []*types.TransactionOrder, owner []byte) map[string]string {
	errs := make(map[string]string)
	var m sync.Mutex

	const maxWorkers = 5
	sem := semaphore.NewWeighted(maxWorkers)
	for _, tx := range txs {
		if err := sem.Acquire(ctx, 1); err != nil {
			break
		}
		go func(tx *types.TransactionOrder) {
			defer sem.Release(1)
			if err := api.saveTx(ctx, tx, owner); err != nil {
				m.Lock()
				errs[hex.EncodeToString(tx.UnitID())] = err.Error()
				m.Unlock()
			}
		}(tx)
	}

	semCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := sem.Acquire(semCtx, maxWorkers); err != nil {
		m.Lock()
		errs["waiting-for-workers"] = err.Error()
		m.Unlock()
	}
	return errs
}

func (api *restAPI) getFeeCreditBill(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitID, err := parseHex[wallet.UnitID](vars["unitId"], true)
	if err != nil {
		api.invalidParamResponse(w, "unitId", err)
		return
	}
	fcb, err := api.db.GetFeeCreditBill(unitID)
	if err != nil {
		api.writeErrorResponse(w, fmt.Errorf("failed to load fee credit bill for ID 0x%X: %w", unitID, err))
		return
	}
	if fcb == nil {
		w.WriteHeader(http.StatusNotFound)
		api.writeResponse(w, ErrorResponse{Message: "fee credit bill does not exist"})
		return
	}
	fcbProof, err := api.db.GetTxProof(unitID, fcb.TxHash)
	if err != nil {
		api.writeErrorResponse(w, fmt.Errorf("failed to load fee credit bill proof for ID 0x%X and TxHash 0x%X: %w", unitID, fcb.GetTxHash(), err))
		return
	}
	if fcbProof == nil {
		w.WriteHeader(http.StatusNotFound)
		api.writeResponse(w, ErrorResponse{Message: "fee credit bill proof does not exist"})
		return
	}
	api.writeResponse(w, fcb.ToGenericBill(fcbProof))
}

func (api *restAPI) saveTx(ctx context.Context, tx *types.TransactionOrder, owner []byte) error {
	// if "creator type tx" then save the type->owner relation
	kind := Any
	switch tx.PayloadType() {
	case tokens.PayloadTypeCreateFungibleTokenType:
		kind = Fungible
	case tokens.PayloadTypeCreateNFTType:
		kind = NonFungible
	}
	if kind != Any {
		if err := api.db.SaveTokenTypeCreator(tx.UnitID(), kind, owner); err != nil {
			return fmt.Errorf("failed to save creator relation: %w", err)
		}
	}

	if err := api.ab.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("failed to forward tx: %w", err)
	}
	return nil
}

func (api *restAPI) getTxProof(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitID, err := parseHex[wallet.UnitID](vars["unitId"], true)
	if err != nil {
		api.invalidParamResponse(w, "unitId", err)
		return
	}
	txHash, err := parseHex[wallet.TxHash](vars["txHash"], true)
	if err != nil {
		api.invalidParamResponse(w, "txHash", err)
		return
	}

	proof, err := api.db.GetTxProof(unitID, txHash)
	if err != nil {
		api.writeErrorResponse(w, fmt.Errorf("failed to load proof of tx 0x%X (unit 0x%X): %w", txHash, unitID, err))
		return
	}
	if proof == nil {
		api.errorResponse(w, http.StatusNotFound, fmt.Errorf("no proof found for tx 0x%X (unit 0x%X)", txHash, unitID))
		return
	}

	api.writeResponse(w, proof)
}

func (api *restAPI) writeResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		api.logError(fmt.Errorf("failed to encode response data as json: %w", err))
	}
}

func (api *restAPI) writeErrorResponse(w http.ResponseWriter, err error) {
	if errors.Is(err, errRecordNotFound) {
		api.errorResponse(w, http.StatusNotFound, err)
		return
	}

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

type (
	RoundNumberResponse struct {
		RoundNumber uint64 `json:"roundNumber,string"`
	}

	ErrorResponse struct {
		Message string `json:"message"`
	}
)
