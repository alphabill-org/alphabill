package twb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
)

func Test_restAPI_endpoints(t *testing.T) {
	t.Parallel()

	/*
		purpose of the test is to make sure that router is built successfully.
		unfortunately it seems that gorilla/mux doesn't fail to build router
		even with invalid paths (ie the same path registered multiple times)
	*/

	api := &restAPI{}
	h := api.endpoints()
	require.NotNil(t, h, "nil handler was returned")

	req := httptest.NewRequest("GET", "http://ab.com/api/v1/foobar", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	rsp := w.Result()
	require.NotNil(t, rsp, "got nil response")
	require.Equal(t, http.StatusNotFound, rsp.StatusCode, "unexpected status")
}

func Test_restAPI_postTransaction(t *testing.T) {
	t.Parallel()

	makeRequest := func(api *restAPI, owner string, body []byte) *http.Response {
		req := httptest.NewRequest("POST", fmt.Sprintf("http://ab.com/api/v1/transactions/%s", owner), bytes.NewReader(body))
		req = mux.SetURLVars(req, map[string]string{"pubkey": owner})
		w := httptest.NewRecorder()
		api.postTransactions(w, req)
		return w.Result()
	}

	// some values to be reused by multiple tests

	// valid owner ID
	ownerID := make([]byte, 33)
	n, err := rand.Read(ownerID)
	require.NoError(t, err)
	require.EqualValues(t, len(ownerID), n)
	ownerIDstr := hexutil.Encode(ownerID)

	// valid request body with single create-nft-type tx
	createNTFTypeTx := randomTx(t, &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "test"})
	createNTFTypeMsg, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(&txsystem.Transactions{Transactions: []*txsystem.Transaction{createNTFTypeTx}})
	require.NoError(t, err)
	require.NotEmpty(t, createNTFTypeMsg)

	t.Run("empty request body", func(t *testing.T) {
		api := &restAPI{} // shouldn't need any of the dependencies
		rsp := makeRequest(api, ownerIDstr, nil)
		if rsp.StatusCode != http.StatusBadRequest {
			t.Errorf("unexpected status %d", rsp.StatusCode)
		}
		// expecting error like
		// failed to decode request body: proto: syntax error (line 1:1): unexpected token
		// but it is not "stable" so check just for the expected beginning
		er := &ErrorResponse{}
		if err := decodeResponse(t, rsp, http.StatusBadRequest, er); err != nil {
			t.Fatal(err.Error())
		}
		require.Contains(t, er.Message, `failed to decode request body`)
	})

	t.Run("empty object in request body", func(t *testing.T) {
		api := &restAPI{} // shouldn't need any of the dependencies
		rsp := makeRequest(api, ownerIDstr, []byte(`{}`))
		if rsp.StatusCode != http.StatusBadRequest {
			t.Errorf("unexpected status %d", rsp.StatusCode)
		}
		expectErrorResponse(t, rsp, http.StatusBadRequest, "request body contained no transactions to process")
	})

	t.Run("failure to convert tx", func(t *testing.T) {
		expErr := fmt.Errorf("can't convert tx")
		var saveTypeCalls, sendTxCalls int32
		api := &restAPI{
			convertTx: func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) { return nil, expErr },
			ab: &mockABClient{
				sendTransaction: func(ctx context.Context, t *txsystem.Transaction) error {
					atomic.AddInt32(&sendTxCalls, 1)
					return fmt.Errorf("unexpected call")
				},
			},
			db: &mockStorage{
				saveTTypeCreator: func(id TokenTypeID, kind Kind, creator PubKey) error {
					atomic.AddInt32(&saveTypeCalls, 1)
					return fmt.Errorf("unexpected call")
				},
			},
		}
		rsp := makeRequest(api, ownerIDstr, createNTFTypeMsg)
		if rsp.StatusCode != http.StatusInternalServerError {
			b, err := httputil.DumpResponse(rsp, true)
			t.Errorf("unexpected status: %s\n%s\n%v", rsp.Status, b, err)
		}
		if saveTypeCalls != 0 {
			t.Errorf("expected no saveTTypeCreator calls but it was called %d times", saveTypeCalls)
		}
		if sendTxCalls != 0 {
			t.Errorf("expected no sendTransaction calls but it was called %d times", sendTxCalls)
		}
	})

	t.Run("one valid type-creation transaction is sent", func(t *testing.T) {
		txsys, err := tokens.New(
			tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
		)
		if err != nil {
			t.Fatalf("failed to create token tx system: %v", err)
		}

		var saveTypeCalls, sendTxCalls int32
		api := &restAPI{
			convertTx: txsys.ConvertTx,
			ab: &mockABClient{
				sendTransaction: func(ctx context.Context, t *txsystem.Transaction) error {
					atomic.AddInt32(&sendTxCalls, 1)
					return nil
				},
			},
			db: &mockStorage{
				saveTTypeCreator: func(id TokenTypeID, kind Kind, creator PubKey) error {
					atomic.AddInt32(&saveTypeCalls, 1)
					if !bytes.Equal(id, createNTFTypeTx.UnitId) {
						t.Errorf("unexpected unit ID %x", id)
					}
					return nil
				},
			},
		}
		rsp := makeRequest(api, ownerIDstr, createNTFTypeMsg)
		if rsp.StatusCode != http.StatusAccepted {
			b, err := httputil.DumpResponse(rsp, true)
			t.Errorf("unexpected status: %s\n%s\n%v", rsp.Status, b, err)
		}
		require.EqualValues(t, 1, saveTypeCalls, "unexpectedly saveTTypeCreator was called %d times", saveTypeCalls)
		require.EqualValues(t, 1, sendTxCalls, "unexpectedly sendTransaction was called %d times", sendTxCalls)
	})

	t.Run("valid non-type-creation request", func(t *testing.T) {
		txs := &txsystem.Transactions{Transactions: []*txsystem.Transaction{
			randomTx(t, &tokens.MintFungibleTokenAttributes{Value: 42}),
		}}
		message, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(txs)
		require.NoError(t, err)

		txsys, err := tokens.New(
			tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
		)
		if err != nil {
			t.Fatalf("failed to create token tx system: %v", err)
		}

		var saveTypeCalls, sendTxCalls int32
		api := &restAPI{
			convertTx: txsys.ConvertTx,
			ab: &mockABClient{
				sendTransaction: func(ctx context.Context, t *txsystem.Transaction) error {
					atomic.AddInt32(&sendTxCalls, 1)
					return nil
				},
			},
			db: &mockStorage{
				saveTTypeCreator: func(id TokenTypeID, kind Kind, creator PubKey) error {
					atomic.AddInt32(&saveTypeCalls, 1)
					return nil
				},
			},
		}
		rsp := makeRequest(api, ownerIDstr, message)
		if rsp.StatusCode != http.StatusAccepted {
			b, err := httputil.DumpResponse(rsp, true)
			t.Errorf("unexpected status: %s\n%s\n%v", rsp.Status, b, err)
		}
		require.EqualValues(t, 0, saveTypeCalls, "unexpectedly saveTTypeCreator was called %d times", saveTypeCalls)
		require.EqualValues(t, 1, sendTxCalls, "unexpectedly sendTransaction was called %d times", sendTxCalls)
	})
}

func Test_restAPI_getToken(t *testing.T) {
	t.Parallel()

	makeRequest := func(api *restAPI, tokenId string) *http.Response {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://ab.com/api/v1/tokens/%s", tokenId), nil)
		req = mux.SetURLVars(req, map[string]string{"tokenId": tokenId})
		w := httptest.NewRecorder()
		api.getToken(w, req)
		return w.Result()
	}

	t.Run("invalid id parameter", func(t *testing.T) {
		api := &restAPI{db: &mockStorage{}}
		rsp := makeRequest(api, "FOOBAR")
		expectErrorResponse(t, rsp, http.StatusBadRequest, `invalid parameter "tokenId": hex string without 0x prefix`)
	})

	t.Run("no token with given id", func(t *testing.T) {
		tokenId := test.RandomBytes(32)
		api := &restAPI{db: &mockStorage{
			getToken: func(id TokenID) (*TokenUnit, error) {
				if !bytes.Equal(id, tokenId) {
					return nil, fmt.Errorf("expected token ID %x got %x", tokenId, id)
				}
				return nil, fmt.Errorf("no token with this id: %w", errRecordNotFound)
			},
		}}
		rsp := makeRequest(api, hexutil.Encode(tokenId))
		expectErrorResponse(t, rsp, http.StatusNotFound, `no token with this id: not found`)
	})

	t.Run("token found", func(t *testing.T) {
		token := &TokenUnit{
			ID:     test.RandomBytes(32),
			Kind:   NonFungible,
			Amount: 42,
		}
		api := &restAPI{db: &mockStorage{
			getToken: func(id TokenID) (*TokenUnit, error) {
				if !bytes.Equal(id, token.ID) {
					return nil, fmt.Errorf("expected token ID %x got %x", token.ID, id)
				}
				return token, nil
			},
		}}
		rsp := makeRequest(api, hexutil.Encode(token.ID))
		var rspData *TokenUnit
		require.NoError(t, decodeResponse(t, rsp, http.StatusOK, &rspData))
		require.Equal(t, token, rspData)
	})
}

func Test_restAPI_listTokens(t *testing.T) {
	t.Parallel()

	ownerID := make([]byte, 33)
	n, err := rand.Read(ownerID)
	require.NoError(t, err)
	require.EqualValues(t, len(ownerID), n)
	pubKeyHex := hexutil.Encode(ownerID)

	t.Run("invalid parameters", func(t *testing.T) {
		cases := []struct {
			kind   string
			owner  string
			qparam string
			errMsg string
		}{
			{kind: "foo", owner: pubKeyHex, qparam: "", errMsg: `invalid parameter "kind": "foo" is not valid token kind`},
			{kind: "all", owner: "", qparam: "", errMsg: `invalid parameter "owner": parameter is required`},
			{kind: "all", owner: "AA", qparam: "", errMsg: `invalid parameter "owner": must be 68 characters long (including 0x prefix), got 2 characters starting AA`},
			{kind: "all", owner: "0x", qparam: "", errMsg: `invalid parameter "owner": must be 68 characters long (including 0x prefix), got 2 characters starting 0x`},
			{kind: "all", owner: "0xAA", qparam: "", errMsg: `invalid parameter "owner": must be 68 characters long (including 0x prefix), got 4 characters starting 0xAA`},
			{kind: "all", owner: "0x" + strings.Repeat("ABCDEFGHIJK", 6), qparam: "", errMsg: `invalid parameter "owner": invalid hex string`}, // correct length but invalid content
			{kind: "all", owner: pubKeyHex, qparam: "offsetKey=ABCDEFGHIJK", errMsg: `invalid parameter "offsetKey": hex string without 0x prefix`},
			{kind: "all", owner: pubKeyHex, qparam: "offsetKey=0xABCDEFGHIJK", errMsg: `invalid parameter "offsetKey": invalid hex string`},
			{kind: "all", owner: pubKeyHex, qparam: "limit=0", errMsg: `invalid parameter "limit": value must be greater than zero, got 0`},
			{kind: "all", owner: pubKeyHex, qparam: "limit=-1", errMsg: `invalid parameter "limit": value must be greater than zero, got -1`},
			{kind: "all", owner: pubKeyHex, qparam: "limit=foo", errMsg: `invalid parameter "limit": failed to parse "foo" as integer: strconv.Atoi: parsing "foo": invalid syntax`},
		}

		api := &restAPI{db: &mockStorage{
			queryTokens: func(kind Kind, owner Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error) {
				t.Error("unexpected QueryTokens call")
				return nil, nil, fmt.Errorf("unexpected QueryTokens(%s, %x, %x, %d) call", kind, owner, startKey, count)
			},
		}}
		makeRequest := func(kind, owner, qparam string) *http.Response {
			req := httptest.NewRequest("GET", fmt.Sprintf("http://ab.com/api/v1/kinds/%s/owners/%s/tokens?%s", kind, owner, qparam), nil)
			req = mux.SetURLVars(req, map[string]string{"kind": kind, "owner": owner})
			w := httptest.NewRecorder()
			api.listTokens(w, req)
			return w.Result()
		}

		for n, tc := range cases {
			t.Run(fmt.Sprintf("test case %d", n), func(t *testing.T) {
				resp := makeRequest(tc.kind, tc.owner, tc.qparam)
				expectErrorResponse(t, resp, http.StatusBadRequest, tc.errMsg)
			})
		}
	})

	makeRequest := func(api *restAPI, kind Kind, owner []byte, qparam string) *http.Response {
		pubKey := hexutil.Encode(owner)
		req := httptest.NewRequest("GET", fmt.Sprintf("http://ab.com/api/v1/kinds/%s/owners/%s/tokens?%s", kind, pubKey, qparam), nil)
		req = mux.SetURLVars(req, map[string]string{"kind": kind.String(), "owner": pubKey})
		w := httptest.NewRecorder()
		api.listTokens(w, req)
		return w.Result()
	}

	t.Run("data is returned by the query and there is more", func(t *testing.T) {
		data := []*TokenUnit{
			{ID: []byte("1111"), Kind: Fungible},
			{ID: []byte("2222"), Kind: NonFungible},
			{ID: []byte("3333"), Kind: Fungible},
		}
		ds := &mockStorage{
			queryTokens: func(kind Kind, owner Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error) {
				require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(hash.Sum256(ownerID)), owner, "unexpected owner key in the query")
				return data, data[len(data)-1].ID, nil
			},
		}

		resp := makeRequest(&restAPI{db: ds}, Any, ownerID, "")
		var rspData []*TokenUnit
		require.NoError(t, decodeResponse(t, resp, http.StatusOK, &rspData))
		require.ElementsMatch(t, data, rspData)

		// because query reported there is more data we must have "Link" header
		var linkHdrMatcher = regexp.MustCompile("<(.*)>")
		match := linkHdrMatcher.FindStringSubmatch(resp.Header.Get("Link"))
		if len(match) != 2 {
			t.Errorf("Link header didn't result in expected match\nHeader: %s\nmatches: %v\n", resp.Header.Get("Link"), match)
		} else {
			u, err := url.Parse(match[1])
			if err != nil {
				t.Fatal("failed to parse Link header:", err)
			}
			exp := encodeHex(data[len(data)-1].ID)
			if s := u.Query().Get("offsetKey"); s != exp {
				t.Errorf("expected %q got %q", exp, s)
			}
		}
	})

	t.Run("no data", func(t *testing.T) {
		ds := &mockStorage{
			queryTokens: func(kind Kind, owner Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error) {
				return nil, nil, nil
			},
		}
		resp := makeRequest(&restAPI{db: ds}, Any, ownerID, "")
		// check that there is no data in body
		var rspData []*TokenUnit
		require.NoError(t, decodeResponse(t, resp, http.StatusOK, &rspData))
		require.Empty(t, rspData)
		// no more data
		if lh := resp.Header.Get("Link"); lh != "" {
			t.Errorf("unexpectedly the Link header is not empty, got %q", lh)
		}
	})
}

func Test_restAPI_listTypes(t *testing.T) {
	t.Parallel()

	makeRequest := func(api *restAPI, kind Kind, qparam string) *http.Response {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://ab.com/api/v1/kinds/%s/types?%s", kind, qparam), nil)
		req = mux.SetURLVars(req, map[string]string{"kind": kind.String()})
		w := httptest.NewRecorder()
		api.listTypes(w, req)
		return w.Result()
	}

	t.Run("invalid query parameter value", func(t *testing.T) {
		cases := []struct {
			qparam string
			errMsg string
		}{
			{
				qparam: "creator=" + hexutil.Encode([]byte{1, 2, 3, 4, 5, 6, 7, 8}),
				errMsg: `invalid parameter "creator": must be 68 characters long (including 0x prefix), got 18 characters starting 0x0102`,
			},
			{
				// correct length but invalid content
				qparam: "creator=0x" + strings.Repeat("ABCDEFGHIJK", 6),
				errMsg: `invalid parameter "creator": invalid hex string`,
			},
			{
				qparam: "offsetKey=01234567890abcdef",
				errMsg: `invalid parameter "offsetKey": hex string without 0x prefix`,
			},
			{
				qparam: "offsetKey=0x" + strings.Repeat("ABCDEFGHIJK", 6),
				errMsg: `invalid parameter "offsetKey": invalid hex string`,
			},
			{qparam: "limit=0", errMsg: `invalid parameter "limit": value must be greater than zero, got 0`},
			{qparam: "limit=-1", errMsg: `invalid parameter "limit": value must be greater than zero, got -1`},
			{qparam: "limit=foo", errMsg: `invalid parameter "limit": failed to parse "foo" as integer: strconv.Atoi: parsing "foo": invalid syntax`},
		}

		api := &restAPI{db: &mockStorage{
			queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
				t.Error("unexpected QueryTokenType call")
				return nil, nil, fmt.Errorf("unexpected QueryTokenType call")
			},
		}}

		for _, tc := range cases {
			resp := makeRequest(api, Any, tc.qparam)
			expectErrorResponse(t, resp, http.StatusBadRequest, tc.errMsg)
		}
	})

	t.Run("limit param is sent to the query", func(t *testing.T) {
		cases := []struct {
			qpar  string // param we send in URL
			value int    // what we expect to see in a query callback
		}{
			{value: 100, qpar: "limit="}, // when param is missing then default value 100 is used
			{value: 1, qpar: "limit=1"},
			{value: 99, qpar: "limit=99"},
			{value: 100, qpar: "limit=100"},
			{value: 100, qpar: "limit=101"}, // when over limit max limit is used
			{value: 100, qpar: "limit=321"},
		}
		for _, tc := range cases {
			ds := &mockStorage{
				queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
					require.Equal(t, tc.value, count, "unexpected count sent to the query func with param %q", tc.qpar)
					return nil, nil, nil
				},
			}
			resp := makeRequest(&restAPI{db: ds}, Any, tc.qpar)
			if resp.StatusCode != http.StatusOK {
				t.Errorf("unexpected status %d for test %q -> %d", resp.StatusCode, tc.qpar, tc.value)
			}
		}
	})

	t.Run("creator param is passed to the query", func(t *testing.T) {
		creatorID := make([]byte, 33)
		n, err := rand.Read(creatorID)
		require.NoError(t, err)
		require.EqualValues(t, len(creatorID), n)

		ds := &mockStorage{
			queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
				require.EqualValues(t, creatorID, creator)
				require.Empty(t, startKey)
				require.True(t, count > 0, "expected count to be > 0, got %d", count)
				return nil, nil, nil
			},
		}
		resp := makeRequest(&restAPI{db: ds}, Any, "creator="+hexutil.Encode(creatorID))
		if resp.StatusCode != http.StatusOK {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})

	t.Run("position param is passed to the query", func(t *testing.T) {
		currentID := make([]byte, 33)
		n, err := rand.Read(currentID)
		require.NoError(t, err)
		require.EqualValues(t, len(currentID), n)

		ds := &mockStorage{
			queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
				require.EqualValues(t, currentID, startKey)
				require.Empty(t, creator)
				require.True(t, count > 0, "expected count to be > 0, got %d", count)
				return nil, nil, nil
			},
		}
		resp := makeRequest(&restAPI{db: ds}, Any, "offsetKey="+encodeHex[TokenTypeID](currentID))
		if resp.StatusCode != http.StatusOK {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})

	t.Run("kind parameter is sent to the query", func(t *testing.T) {
		for _, tc := range []Kind{Any, Fungible, NonFungible} {
			ds := &mockStorage{
				queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
					require.Equal(t, kind, tc, "unexpected token kind sent to the query func")
					return nil, nil, nil
				},
			}
			resp := makeRequest(&restAPI{db: ds}, tc, "")
			if resp.StatusCode != http.StatusOK {
				t.Errorf("unexpected status %d for kind %d", resp.StatusCode, tc)
			}
		}
	})

	t.Run("invalid kind parameter is sent", func(t *testing.T) {
		ds := &mockStorage{
			queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
				t.Error("unexpected queryTTypes call")
				return nil, nil, nil
			},
		}
		api := &restAPI{db: ds}

		req := httptest.NewRequest("GET", "http://ab.com/api/v1/kinds/foo/types", nil)
		req = mux.SetURLVars(req, map[string]string{"kind": "foo"})
		w := httptest.NewRecorder()

		api.listTypes(w, req)

		expectErrorResponse(t, w.Result(), http.StatusBadRequest, `invalid parameter "kind": "foo" is not valid token kind`)
	})

	t.Run("query returns error", func(t *testing.T) {
		expErr := fmt.Errorf("failed to query database")
		ds := &mockStorage{
			queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
				return nil, nil, expErr
			},
		}

		resp := makeRequest(&restAPI{db: ds}, Any, "")
		expectErrorResponse(t, resp, http.StatusInternalServerError, `failed to query database`)
	})

	t.Run("no data matches the query", func(t *testing.T) {
		ds := &mockStorage{
			queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
				return nil, nil, nil
			},
		}
		resp := makeRequest(&restAPI{db: ds}, Any, "")
		// check that there is nothing in body
		var rspData []*TokenUnitType
		require.NoError(t, decodeResponse(t, resp, http.StatusOK, &rspData))
		require.Empty(t, rspData)
		// no more data
		if lh := resp.Header.Get("Link"); lh != "" {
			t.Errorf("unexpectedly the Link header is not empty, got %q", lh)
		}
	})

	t.Run("data is returned by the query and there is more", func(t *testing.T) {
		data := []*TokenUnitType{
			{ID: []byte("1111"), Kind: Fungible},
			{ID: []byte("2222"), Kind: NonFungible},
			{ID: []byte("3333"), Kind: Fungible},
		}
		ds := &mockStorage{
			queryTTypes: func(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
				return data, data[len(data)-1].ID, nil
			},
		}

		resp := makeRequest(&restAPI{db: ds}, Any, "")
		var rspData []*TokenUnitType
		require.NoError(t, decodeResponse(t, resp, http.StatusOK, &rspData))
		require.ElementsMatch(t, data, rspData)

		// because query reported there is more data we must have "Link" header
		var linkHdrMatcher = regexp.MustCompile("<(.*)>")
		match := linkHdrMatcher.FindStringSubmatch(resp.Header.Get("Link"))
		if len(match) != 2 {
			t.Errorf("Link header didn't result in expected match\nHeader: %s\nmatches: %v\n", resp.Header.Get("Link"), match)
		} else {
			u, err := url.Parse(match[1])
			if err != nil {
				t.Fatal("failed to parse Link header:", err)
			}
			exp := encodeHex(data[len(data)-1].ID)
			if s := u.Query().Get("offsetKey"); s != exp {
				t.Errorf("expected %q got %q", exp, s)
			}
		}
	})
}

func Test_restAPI_typeHierarchy(t *testing.T) {
	t.Parallel()

	makeRequest := func(api *restAPI, typeId string) *http.Response {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://ab.com/api/v1/types/%s/hierarchy", typeId), nil)
		req = mux.SetURLVars(req, map[string]string{"typeId": typeId})
		w := httptest.NewRecorder()
		api.typeHierarchy(w, req)
		return w.Result()
	}

	t.Run("invalid input params", func(t *testing.T) {
		rsp := makeRequest(&restAPI{}, "foobar")
		expectErrorResponse(t, rsp, http.StatusBadRequest, `invalid parameter "typeId": hex string without 0x prefix`)
	})

	t.Run("no type with given id", func(t *testing.T) {
		api := &restAPI{
			db: &mockStorage{
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					return nil, fmt.Errorf("no such type: %w", errRecordNotFound)
				},
			},
		}
		typeId := "0x0001"
		rsp := makeRequest(api, typeId)
		expectErrorResponse(t, rsp, http.StatusNotFound, fmt.Sprintf(`failed to load type with id %s: no such type: not found`, typeId[2:]))
	})

	t.Run("type is root", func(t *testing.T) {
		tokTyp := randomTokenType(NonFungible)
		tokTyp.ParentTypeID = nil
		api := &restAPI{
			db: &mockStorage{
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					if bytes.Equal(id, tokTyp.ID) {
						return tokTyp, nil
					}
					return nil, fmt.Errorf("unexpected type id %x", id)
				},
			},
		}

		rsp := makeRequest(api, encodeHex(tokTyp.ID))
		var rspData []*TokenUnitType
		require.NoError(t, decodeResponse(t, rsp, http.StatusOK, &rspData))
		require.ElementsMatch(t, rspData, []*TokenUnitType{tokTyp})
	})

	t.Run("type is root, parent typeID == 0x00", func(t *testing.T) {
		tokTyp := randomTokenType(NonFungible)
		tokTyp.ParentTypeID = NoParent
		api := &restAPI{
			db: &mockStorage{
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					if bytes.Equal(id, tokTyp.ID) {
						return tokTyp, nil
					}
					return nil, fmt.Errorf("unexpected type id %x", id)
				},
			},
		}

		rsp := makeRequest(api, encodeHex(tokTyp.ID))
		var rspData []*TokenUnitType
		require.NoError(t, decodeResponse(t, rsp, http.StatusOK, &rspData))
		require.ElementsMatch(t, rspData, []*TokenUnitType{tokTyp})
	})

	t.Run("type has one parent", func(t *testing.T) {
		tokTypA := randomTokenType(NonFungible)
		tokTypA.ParentTypeID = nil

		tokTypB := randomTokenType(NonFungible)
		tokTypB.ParentTypeID = tokTypA.ID

		api := &restAPI{
			db: &mockStorage{
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					switch {
					case bytes.Equal(id, tokTypA.ID):
						return tokTypA, nil
					case bytes.Equal(id, tokTypB.ID):
						return tokTypB, nil
					}
					return nil, fmt.Errorf("unexpected type id %x", id)
				},
			},
		}

		rsp := makeRequest(api, encodeHex(tokTypB.ID))
		var rspData []*TokenUnitType
		require.NoError(t, decodeResponse(t, rsp, http.StatusOK, &rspData))
		require.ElementsMatch(t, rspData, []*TokenUnitType{tokTypA, tokTypB})
	})
}

func Test_restAPI_getRoundNumber(t *testing.T) {
	t.Parallel()

	makeRequest := func(api *restAPI) *http.Response {
		req := httptest.NewRequest("GET", "http://ab.com/api/v1/round-number", nil)
		w := httptest.NewRecorder()
		api.getRoundNumber(w, req)
		return w.Result()
	}

	t.Run("query returns error", func(t *testing.T) {
		expErr := fmt.Errorf("failed to read round number")
		abc := &mockABClient{
			roundNumber: func(context.Context) (uint64, error) { return 0, expErr },
		}
		resp := makeRequest(&restAPI{ab: abc})
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})

	t.Run("success", func(t *testing.T) {
		abc := &mockABClient{
			roundNumber: func(context.Context) (uint64, error) { return 42, nil },
		}
		resp := makeRequest(&restAPI{ab: abc})
		if resp.StatusCode != http.StatusOK {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
		defer resp.Body.Close()

		rnr := &RoundNumberResponse{}
		if err := json.NewDecoder(resp.Body).Decode(rnr); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}
		if rnr.RoundNumber != 42 {
			t.Errorf("expected response to be 42, got %d", rnr.RoundNumber)
		}
	})
}

func Test_restAPI_txProof(t *testing.T) {
	t.Parallel()

	makeRequest := func(api *restAPI, unitID, txHash string) *http.Response {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://ab.com/api/v1/units/%s/transactions/%s/proof", unitID, txHash), nil)
		req = mux.SetURLVars(req, map[string]string{"unitId": unitID, "txHash": txHash})
		w := httptest.NewRecorder()
		api.getTxProof(w, req)
		return w.Result()
	}

	t.Run("invalid 'unitId' input params", func(t *testing.T) {
		rsp := makeRequest(&restAPI{}, "foo", "bar")
		expectErrorResponse(t, rsp, http.StatusBadRequest, `invalid parameter "unitId": hex string without 0x prefix`)
	})

	t.Run("invalid 'txHash' input params", func(t *testing.T) {
		rsp := makeRequest(&restAPI{}, "0x01", "bar")
		expectErrorResponse(t, rsp, http.StatusBadRequest, `invalid parameter "txHash": hex string without 0x prefix`)
	})

	t.Run("error fetching proof", func(t *testing.T) {
		api := &restAPI{
			db: &mockStorage{
				getTxProof: func(unitID UnitID, txHash TxHash) (*Proof, error) {
					return nil, errors.New("error fetching proof")
				},
			},
		}
		rsp := makeRequest(api, "0x01", "0xFF")
		expectErrorResponse(t, rsp, http.StatusInternalServerError, "failed to load proof of tx 0xFF (unit 0x01): error fetching proof")
	})

	t.Run("no proof with given inputs", func(t *testing.T) {
		api := &restAPI{
			db: &mockStorage{
				getTxProof: func(unitID UnitID, txHash TxHash) (*Proof, error) {
					return nil, nil
				},
			},
		}
		rsp := makeRequest(api, "0x01", "0xFF")
		expectErrorResponse(t, rsp, http.StatusNotFound, "no proof found for tx 0xFF (unit 0x01)")
	})

	t.Run("proof is fetched", func(t *testing.T) {
		unitID := []byte{0x01}
		txHash := []byte{0xFF}

		proof := &Proof{1, &txsystem.Transaction{UnitId: unitID}, &block.BlockProof{TransactionsHash: txHash}}
		api := &restAPI{
			db: &mockStorage{
				getTxProof: func(unitID UnitID, txHash TxHash) (*Proof, error) {
					return proof, nil
				},
			},
		}
		rsp := makeRequest(api, encodeHex[UnitID](unitID), encodeHex[TxHash](txHash))
		require.Equal(t, http.StatusOK, rsp.StatusCode, "unexpected status")

		defer rsp.Body.Close()

		proofFromApi := &Proof{}
		if err := json.NewDecoder(rsp.Body).Decode(proofFromApi); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}

		require.Equal(t, proof, proofFromApi)
	})
}

func Test_restAPI_getFeeCreditBill(t *testing.T) {
	t.Parallel()

	makeRequest := func(api *restAPI, unitID string) *http.Response {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://ab.com/api/v1/fee-credit-bill?bill_id=%s", unitID), nil)
		w := httptest.NewRecorder()
		api.getFeeCreditBill(w, req)
		return w.Result()
	}

	t.Run("400 'bill_id' query param not in hex format", func(t *testing.T) {
		rsp := makeRequest(&restAPI{}, "foo")
		expectErrorResponse(t, rsp, http.StatusBadRequest, `invalid parameter "bill_id": hex string without 0x prefix`)
	})

	t.Run("500 error fetching fee credit bill", func(t *testing.T) {
		api := &restAPI{
			db: &mockStorage{
				getFeeCreditBill: func(unitID UnitID) (*FeeCreditBill, error) {
					return nil, errors.New("error fetching fee credit bill")
				},
			},
		}
		rsp := makeRequest(api, "0x01")
		expectErrorResponse(t, rsp, http.StatusInternalServerError, "failed to load fee credit bill for ID 0x01: error fetching fee credit bill")
	})

	t.Run("404 fee credit bill not found", func(t *testing.T) {
		api := &restAPI{
			db: &mockStorage{
				getFeeCreditBill: func(unitID UnitID) (*FeeCreditBill, error) {
					return nil, nil
				},
			},
		}
		rsp := makeRequest(api, "0x01")
		expectErrorResponse(t, rsp, http.StatusNotFound, "bill does not exist")
	})

	t.Run("ok", func(t *testing.T) {
		fcb := &FeeCreditBill{
			Id:            []byte{1},
			Value:         2,
			TxHash:        []byte{3},
			FCBlockNumber: 4,
		}
		api := &restAPI{
			db: &mockStorage{
				getFeeCreditBill: func(unitID UnitID) (*FeeCreditBill, error) {
					return fcb, nil
				},
			},
		}
		rsp := makeRequest(api, encodeHex[UnitID](fcb.Id))
		require.Equal(t, http.StatusOK, rsp.StatusCode, "unexpected status")
		defer rsp.Body.Close()

		fcbFromAPI := &FeeCreditBill{}
		if err := json.NewDecoder(rsp.Body).Decode(fcbFromAPI); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}

		require.Equal(t, fcb, fcbFromAPI)
	})
}
