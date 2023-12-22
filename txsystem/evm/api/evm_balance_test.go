package api

import (
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc"
	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm/conversion"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/evm/unit"
	fc "github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestAPI_Balance_OK(t *testing.T) {
	tree := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(tree, logger.New(t))
	address := common.BytesToAddress(test.RandomBytes(20))
	// simulate EVM SC created account, not backlink
	stateDB.CreateAccount(address)
	balance := big.NewInt(101)
	stateDB.AddBalance(address, balance)
	_, _, err := tree.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, tree.Commit())

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", address.Bytes()), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), logger.NOP(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_        struct{} `cbor:",toarray"`
		Balance  string
		Backlink []byte
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Equal(t, balance.String(), resp.Balance)
	require.Len(t, resp.Backlink, 0)
}

func TestAPI_BalanceWithBacklink_OK(t *testing.T) {
	s := abstate.NewEmptyState()
	// simulate add credit
	address := common.BytesToAddress(test.RandomBytes(20))
	unitID := unit.NewEvmAccountIDFromAddress(address)
	var balance uint64 = 101
	backlink := test.RandomBytes(32)
	err := s.Apply(fc.AddCredit(unitID, templates.AlwaysTrueBytes(), unit.NewEvmFcr(balance, backlink, 10)))
	require.NoError(t, err)
	_, _, err = s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	a := &API{
		state:            s,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", address.Bytes()), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), logger.NOP(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_        struct{} `cbor:",toarray"`
		Balance  string
		Backlink []byte
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Equal(t, conversion.AlphaToWei(balance).String(), resp.Balance)
	require.Equal(t, backlink, resp.Backlink)
}

func TestAPI_Balance_NotFound(t *testing.T) {
	a := &API{
		state:            abstate.NewEmptyState(),
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(10),
		blockGasLimit:    10000,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/evm/balance/%X", test.RandomBytes(20)), nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), logger.NOP(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
