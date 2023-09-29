package backend

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
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
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/broker"
)

type dataSource interface {
	GetTokenType(id TokenTypeID) (*TokenUnitType, error)
	QueryTokenType(kind Kind, creator sdk.PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error)
	GetToken(id TokenID) (*TokenUnit, error)
	QueryTokens(kind Kind, owner sdk.Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error)
	SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator sdk.PubKey) error
	GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)
	GetFeeCreditBill(unitID types.UnitID) (*FeeCreditBill, error)
	GetClosedFeeCredit(fcbID types.UnitID) (*types.TransactionRecord, error)
}

type abClient interface {
	SendTransaction(ctx context.Context, tx *types.TransactionOrder) error
	GetRoundNumber(ctx context.Context) (uint64, error)
}

type tokensRestAPI struct {
	db        dataSource
	ab        abClient
	streamSSE func(ctx context.Context, owner broker.PubKey, w http.ResponseWriter) error
	rw        sdk.ResponseWriter
	systemID  []byte
}

const maxResponseItems = 100

func (api *tokensRestAPI) endpoints() http.Handler {
	apiRouter := mux.NewRouter().StrictSlash(true).PathPrefix("/api").Subrouter()

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
	apiV1.HandleFunc("/tokens/{tokenId}", api.getToken).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/types/{typeId}/hierarchy", api.typeHierarchy).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/kinds/{kind}/owners/{owner}/tokens", api.listTokens).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/kinds/{kind}/types", api.listTypes).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/round-number", api.getRoundNumber).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/transactions/{pubkey}", api.postTransactions).Methods("POST", "OPTIONS")
	apiV1.HandleFunc("/events/{pubkey}/subscribe", api.subscribeEvents).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/units/{unitId}/transactions/{txHash}/proof", api.getTxProof).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/fee-credit-bills/{unitId}", api.getFeeCreditBill).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/closed-fee-credit/{unitId}", api.getClosedFeeCredit).Methods("GET", "OPTIONS")
	apiV1.HandleFunc("/info", api.getInfo).Methods("GET", "OPTIONS")

	apiV1.Handle("/swagger/swagger-initializer.js", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initializer := "swagger/swagger-initializer-tokens.js"
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

	return apiRouter
}

func (api *tokensRestAPI) listTokens(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner, err := sdk.ParsePubKey(vars["owner"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "owner", err)
		return
	}

	kind, err := strToTokenKind(vars["kind"])
	if err != nil {
		api.rw.InvalidParamResponse(w, "kind", err)
		return
	}

	qp := r.URL.Query()
	startKey, err := sdk.ParseHex[types.UnitID](qp.Get(sdk.QueryParamOffsetKey), false)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamOffsetKey, err)
		return
	}

	limit, err := sdk.ParseMaxResponseItems(qp.Get(sdk.QueryParamLimit), maxResponseItems)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamLimit, err)
		return
	}

	data, next, err := api.db.QueryTokens(
		kind,
		script.PredicatePayToPublicKeyHashDefault(hash.Sum256(owner)),
		TokenID(startKey),
		limit)
	if err != nil {
		api.rw.WriteErrorResponse(w, err)
		return
	}
	sdk.SetLinkHeader(r.URL, w, sdk.EncodeHex(next))
	api.rw.WriteResponse(w, data)
}

func (api *tokensRestAPI) getToken(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tokenId, err := sdk.ParseHex[types.UnitID](vars["tokenId"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "tokenId", err)
		return
	}

	token, err := api.db.GetToken(TokenID(tokenId))
	if err != nil {
		api.rw.WriteErrorResponse(w, err)
		return
	}
	api.rw.WriteResponse(w, token)
}

func (api *tokensRestAPI) listTypes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	kind, err := strToTokenKind(vars["kind"])
	if err != nil {
		api.rw.InvalidParamResponse(w, "kind", err)
		return
	}

	qp := r.URL.Query()
	creator, err := sdk.ParsePubKey(qp.Get("creator"), false)
	if err != nil {
		api.rw.InvalidParamResponse(w, "creator", err)
		return
	}

	startKey, err := sdk.ParseHex[types.UnitID](qp.Get(sdk.QueryParamOffsetKey), false)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamOffsetKey, err)
		return
	}

	limit, err := sdk.ParseMaxResponseItems(qp.Get(sdk.QueryParamLimit), maxResponseItems)
	if err != nil {
		api.rw.InvalidParamResponse(w, sdk.QueryParamLimit, err)
		return
	}

	data, next, err := api.db.QueryTokenType(kind, creator, TokenTypeID(startKey), limit)
	if err != nil {
		api.rw.WriteErrorResponse(w, err)
		return
	}
	sdk.SetLinkHeader(r.URL, w, sdk.EncodeHex(next))
	api.rw.WriteResponse(w, data)
}

func (api *tokensRestAPI) typeHierarchy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	typeId, err := sdk.ParseHex[types.UnitID](vars["typeId"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "typeId", err)
		return
	}

	var rsp []*TokenUnitType
	for len(typeId) > 0 && !bytes.Equal(typeId, NoParent) {
		tokTyp, err := api.db.GetTokenType(TokenTypeID(typeId))
		if err != nil {
			api.rw.WriteErrorResponse(w, fmt.Errorf("failed to load type with id %s: %w", typeId, err))
			return
		}
		rsp = append(rsp, tokTyp)
		typeId = types.UnitID(tokTyp.ParentTypeID)
	}
	api.rw.WriteResponse(w, rsp)
}

func (api *tokensRestAPI) getRoundNumber(w http.ResponseWriter, r *http.Request) {
	rn, err := api.ab.GetRoundNumber(r.Context())
	if err != nil {
		api.rw.WriteErrorResponse(w, err)
		return
	}
	api.rw.WriteResponse(w, RoundNumberResponse{RoundNumber: rn})
}

func (api *tokensRestAPI) subscribeEvents(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ownerPK, err := sdk.ParsePubKey(vars["pubkey"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "pubkey", err)
		return
	}

	if err := api.streamSSE(r.Context(), broker.PubKey(ownerPK), w); err != nil {
		api.rw.WriteErrorResponse(w, fmt.Errorf("event streaming failed: %w", err))
	}
}

func (api *tokensRestAPI) postTransactions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to read request body: %w", err))
		return
	}

	vars := mux.Vars(r)
	owner, err := sdk.ParsePubKey(vars["pubkey"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "pubkey", err)
		return
	}

	txs := &sdk.Transactions{}
	if err = cbor.Unmarshal(buf, txs); err != nil {
		api.rw.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("failed to decode request body: %w", err))
		return
	}
	if len(txs.Transactions) == 0 {
		api.rw.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("request body contained no transactions to process"))
		return
	}

	if errs := api.saveTxs(r.Context(), txs.Transactions, owner); len(errs) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		api.rw.WriteResponse(w, errs)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (api *tokensRestAPI) saveTxs(ctx context.Context, txs []*types.TransactionOrder, owner []byte) map[string]string {
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

func (api *tokensRestAPI) getFeeCreditBill(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitID, err := sdk.ParseHex[types.UnitID](vars["unitId"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "unitId", err)
		return
	}
	fcb, err := api.db.GetFeeCreditBill(unitID)
	if err != nil {
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to load fee credit bill for ID 0x%s: %w", unitID, err))
		return
	}
	if fcb == nil {
		w.WriteHeader(http.StatusNotFound)
		api.rw.WriteResponse(w, sdk.ErrorResponse{Message: "fee credit bill does not exist"})
		return
	}
	api.rw.WriteResponse(w, fcb.ToGenericBill())
}

func (api *tokensRestAPI) getClosedFeeCredit(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitID, err := sdk.ParseHex[types.UnitID](vars["unitId"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "unitId", err)
		return
	}
	cfc, err := api.db.GetClosedFeeCredit(unitID)
	if err != nil {
		api.rw.WriteErrorResponse(w, fmt.Errorf("failed to load closed fee credit for ID 0x%s: %w", unitID, err))
		return
	}
	if cfc == nil {
		w.WriteHeader(http.StatusNotFound)
		api.rw.WriteResponse(w, sdk.ErrorResponse{Message: "closed fee credit does not exist"})
		return
	}
	api.rw.WriteCborResponse(w, cfc)
}

func (api *tokensRestAPI) getInfo(w http.ResponseWriter, _ *http.Request) {
	systemID := hex.EncodeToString(api.systemID)
	res := sdk.InfoResponse{
		SystemID: systemID,
		Name:     "tokens backend",
	}
	api.rw.WriteResponse(w, res)
}

func (api *tokensRestAPI) saveTx(ctx context.Context, tx *types.TransactionOrder, owner []byte) error {
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

func (api *tokensRestAPI) getTxProof(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitID, err := sdk.ParseHex[types.UnitID](vars["unitId"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "unitId", err)
		return
	}
	txHash, err := sdk.ParseHex[sdk.TxHash](vars["txHash"], true)
	if err != nil {
		api.rw.InvalidParamResponse(w, "txHash", err)
		return
	}

	proof, err := api.db.GetTxProof(unitID, txHash)
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

type (
	RoundNumberResponse struct {
		RoundNumber uint64 `json:"roundNumber,string"`
	}
)
