package api

import (
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/holiman/uint256"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	teststate "github.com/alphabill-org/alphabill/internal/testutils/state"
	"github.com/alphabill-org/alphabill/rpc"
	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestAPI_Balance_OK(t *testing.T) {
	tree := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(tree, logger.New(t))
	address := common.BytesToAddress(test.RandomBytes(20))

	stateDB.CreateAccount(address)
	balance := uint256.NewInt(101)
	stateDB.AddBalance(address, balance, tracing.BalanceChangeUnspecified)
	teststate.CommitWithUC(t, tree)

	a := &API{
		state:         tree,
		partitionID:   1,
		gasUnitPrice:  big.NewInt(10),
		blockGasLimit: 10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", address.Bytes()), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_       struct{} `cbor:",toarray"`
		Balance string
		Counter uint64
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, balance.String(), resp.Balance)
	require.EqualValues(t, 0, resp.Counter)
}

func TestAPI_BalanceWithCounter(t *testing.T) {
	tree := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(tree, logger.New(t))
	address := common.BytesToAddress(test.RandomBytes(20))

	stateDB.CreateAccount(address)
	balance := uint256.NewInt(101)
	stateDB.AddBalance(address, balance, tracing.BalanceChangeUnspecified)
	counter := uint64(10)
	stateDB.SetAlphaBillData(address, &statedb.AlphaBillLink{
		Counter: counter,
	})
	teststate.CommitWithUC(t, tree)

	a := &API{
		state:         tree,
		partitionID:   1,
		gasUnitPrice:  big.NewInt(10),
		blockGasLimit: 10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", address.Bytes()), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_       struct{} `cbor:",toarray"`
		Balance string
		Counter uint64
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, balance.String(), resp.Balance)
	require.Equal(t, counter, resp.Counter)
}

func TestAPI_Balance_NotFound(t *testing.T) {
	a := &API{
		state:         abstate.NewEmptyState(),
		partitionID:   1,
		gasUnitPrice:  big.NewInt(10),
		blockGasLimit: 10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", test.RandomBytes(20)), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
