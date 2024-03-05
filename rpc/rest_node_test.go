package rpc

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/partition"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
)

func TestRestServer_SubmitTransaction(t *testing.T) {
	node := &MockNode{}
	transaction := createTxOrder(t)
	message, err := cbor.Marshal(transaction)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(message))
	recorder := httptest.NewRecorder()

	obs := observability.Default(t)
	NewRESTServer("", MaxBodySize, obs, NodeEndpoints(node, nil, obs)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Equal(t, 1, len(node.transactions))
	tx := node.transactions[0]
	require.Equal(t, transaction.SystemID(), tx.SystemID())
	require.Equal(t, transaction.UnitID(), tx.UnitID())
	require.Equal(t, transaction.OwnerProof, tx.OwnerProof)
	require.Equal(t, transaction.Timeout(), tx.Timeout())
	require.Equal(t, transaction.PayloadType(), tx.PayloadType())
	require.Equal(t, transaction.Payload.Attributes, tx.Payload.Attributes)
}

func createTxOrder(t *testing.T) *types.TransactionOrder {
	transaction := testtransaction.NewTransactionOrder(t,
		testtransaction.WithAttributes([]byte{0, 0, 0, 0, 0, 0, 0}),
		testtransaction.WithUnitId([]byte{0, 0, 0, 1}),
		testtransaction.WithSystemID(1),
		testtransaction.WithOwnerProof([]byte{0, 0, 0, 2}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: 100}),
		testtransaction.WithPayloadType("test"),
	)
	return transaction
}

func TestNewRESTServer_InvalidTx(t *testing.T) {
	// CBOR decoder is returns ok status if byte is 0xF6 (null) or 0xF7 (undefined),
	// in both cases all Tx fields are still nil, but this is not checked in mock, hence the telegram gets accepted
	// Do not use random bytes, use something that will always fail.
	// In real situation the Tx validator would reject just 1 byte Tx anyway, here it is important to just simulate an error
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader([]byte{0x00}))
	recorder := httptest.NewRecorder()

	obs := observability.Default(t)
	NewRESTServer("", MaxBodySize, obs, NodeEndpoints(&MockNode{}, nil, obs)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "unable to decode request body as transaction")
}

func TestRESTServer_GetLatestRoundNumber(t *testing.T) {
	node := &MockNode{}
	obs := observability.Default(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rounds/latest", bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, NodeEndpoints(node, nil, obs)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
	var response uint64
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(&response))
	require.Equal(t, node.maxBlockNumber, response)
}

func TestRESTServer_GetTransactionRecord_OK(t *testing.T) {
	obs := observability.Default(t)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/transactions/%s", hex.EncodeToString(test.RandomBytes(32))), bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, NodeEndpoints(&MockNode{}, nil, obs)).Handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
	require.Equal(t, applicationCBOR, recorder.Result().Header.Get(headerContentType))

	response := struct {
		_        struct{} `cbor:",toarray"`
		TxRecord *types.TransactionRecord
		TxProof  *types.TxProof
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(&response))
	require.NotNil(t, response.TxRecord)
	require.NotNil(t, response.TxProof)
}

func TestRESTServer_GetTransactionRecord_NotFound(t *testing.T) {
	var hash [32]byte
	obs := observability.Default(t)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/transactions/%s", hex.EncodeToString(hash[:])), bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, NodeEndpoints(&MockNode{err: partition.ErrIndexNotFound}, nil, obs)).Handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Result().StatusCode)
	require.Equal(t, int64(-1), recorder.Result().ContentLength)
}

func TestRESTServer_GetOwnerUnits(t *testing.T) {
	t.Run("get owner units - server error", func(t *testing.T) {
		var hash [32]byte
		obs := observability.Default(t)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/owner-unit-ids/%s", hex.EncodeToString(hash[:])), bytes.NewReader([]byte{}))
		recorder := httptest.NewRecorder()
		NewRESTServer("", 10, obs, NodeEndpoints(&MockNode{err: fmt.Errorf("something is wrong")}, nil, obs)).Handler.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusInternalServerError, recorder.Result().StatusCode)
		require.Contains(t, recorder.Body.String(), "something is wrong")
		require.Equal(t, int64(-1), recorder.Result().ContentLength)
	})
	t.Run("get owner units - empty", func(t *testing.T) {
		var hash [32]byte
		ownerID := hex.EncodeToString(hash[:])
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/owner-unit-ids/%s", ownerID), bytes.NewReader([]byte{}))
		recorder := httptest.NewRecorder()
		obs := observability.Default(t)
		NewRESTServer("", 10, obs, NodeEndpoints(&MockNode{ownerUnits: map[string][]types.UnitID{}}, nil, obs)).Handler.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
		require.Equal(t, applicationCBOR, recorder.Result().Header.Get(headerContentType))
		var response []types.UnitID
		require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(&response))
		require.Nil(t, response, "expected response body to be cbor nil (f6)")
	})
	t.Run("get owner units - ok", func(t *testing.T) {
		var hash [32]byte
		obs := observability.Default(t)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/owner-unit-ids/%s", hex.EncodeToString(hash[:])), bytes.NewReader([]byte{}))
		recorder := httptest.NewRecorder()
		mockNode := &MockNode{
			ownerUnits: map[string][]types.UnitID{string(hash[:]): {types.UnitID{0}, types.UnitID{1}}},
		}
		NewRESTServer("", 10, obs, NodeEndpoints(mockNode, nil, obs)).Handler.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
		require.Equal(t, applicationCBOR, recorder.Result().Header.Get(headerContentType))
		var response []types.UnitID
		require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(&response))
		require.NotNil(t, response)
		require.Len(t, response, 2)
		require.Equal(t, types.UnitID{0}, response[0])
		require.Equal(t, types.UnitID{1}, response[1])
	})
}
