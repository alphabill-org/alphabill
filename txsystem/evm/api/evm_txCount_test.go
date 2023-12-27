package api

import (
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	teststate "github.com/alphabill-org/alphabill/internal/testutils/state"
	"github.com/alphabill-org/alphabill/rpc"
	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestAPI_TransactionCount_OK(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(tree, observe.Logger())
	address := common.BytesToAddress(test.RandomBytes(20))

	stateDB.CreateAccount(address)
	stateDB.SetNonce(address, 333)
	teststate.CommitWithUC(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
		log:              observe.Logger(),
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/transactionCount/%X", address.Bytes()), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_     struct{} `cbor:",toarray"`
		Nonce uint64
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.EqualValues(t, 333, resp.Nonce)
}

func TestAPI_TransactionCount_NotFound(t *testing.T) {
	a := &API{
		state:            abstate.NewEmptyState(),
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/transactionCount/%X", test.RandomBytes(20)), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
