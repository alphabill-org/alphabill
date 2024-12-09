package api

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/stretchr/testify/require"
)

func TestAPI_GasPrice(t *testing.T) {
	a := &API{
		state:               nil,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(evm.DefaultGasPrice),
		blockGasLimit:       10000,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/evm/gasPrice", nil)
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &struct {
		_        struct{} `cbor:",toarray"`
		GasPrice string
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, a.gasUnitPrice.String(), resp.GasPrice)
}
