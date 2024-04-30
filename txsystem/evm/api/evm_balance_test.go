package api

import (
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/types"

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
	balance := big.NewInt(101)
	stateDB.AddBalance(address, balance)
	teststate.CommitWithUC(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: 1,
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", address.Bytes()), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_        struct{} `cbor:",toarray"`
		Balance  string
		Backlink []byte
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, balance.String(), resp.Balance)
	require.Len(t, resp.Backlink, 0)
}

func TestAPI_BalanceWithBacklink(t *testing.T) {
	tree := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(tree, logger.New(t))
	address := common.BytesToAddress(test.RandomBytes(20))

	stateDB.CreateAccount(address)
	balance := big.NewInt(101)
	stateDB.AddBalance(address, balance)
	backlink := test.RandomBytes(20)
	stateDB.SetAlphaBillData(address, &statedb.AlphaBillLink{
		TxHash: backlink,
	})
	teststate.CommitWithUC(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: 1,
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", address.Bytes()), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_        struct{} `cbor:",toarray"`
		Balance  string
		Backlink []byte
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, balance.String(), resp.Balance)
	require.Equal(t, backlink, resp.Backlink)
}

func TestAPI_Balance_NotFound(t *testing.T) {
	a := &API{
		state:            abstate.NewEmptyState(),
		systemIdentifier: 1,
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", test.RandomBytes(20)), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
