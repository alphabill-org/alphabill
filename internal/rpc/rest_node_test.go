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
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
)

func TestRestServer_SubmitTransaction(t *testing.T) {
	node := &MockNode{}
	transaction := createTxOrder(t)
	message, err := cbor.Marshal(transaction)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(message))
	recorder := httptest.NewRecorder()

	log := logger.New(t)
	obs := observability.NOPMetrics()
	NewRESTServer("", MaxBodySize, obs, log, NodeEndpoints(node, obs, log)).Handler.ServeHTTP(recorder, req)
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
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
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

	log := logger.New(t)
	obs := observability.NOPMetrics()
	NewRESTServer("", MaxBodySize, obs, log, NodeEndpoints(&MockNode{}, obs, log)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "unable to decode request body as transaction")
}

func TestRESTServer_GetLatestRoundNumber(t *testing.T) {
	node := &MockNode{}
	log := logger.New(t)
	obs := observability.NOPMetrics()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rounds/latest", bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, log, NodeEndpoints(node, obs, log)).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Result().StatusCode)
	var response uint64
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(&response))
	require.Equal(t, node.maxBlockNumber, response)
}

func TestRESTServer_GetTransactionRecord_OK(t *testing.T) {
	log := logger.New(t)
	obs := observability.NOPMetrics()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/transactions/%s", hex.EncodeToString(test.RandomBytes(32))), bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, log, NodeEndpoints(&MockNode{}, obs, log)).Handler.ServeHTTP(recorder, req)

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
	log := logger.New(t)
	obs := observability.NOPMetrics()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/transactions/%s", hex.EncodeToString(hash[:])), bytes.NewReader([]byte{}))
	recorder := httptest.NewRecorder()
	NewRESTServer("", 10, obs, log, NodeEndpoints(&MockNode{}, obs, log)).Handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Result().StatusCode)
	require.Equal(t, int64(-1), recorder.Result().ContentLength)
}
