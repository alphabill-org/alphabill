package api

import (
	"bytes"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rpc"
	abstate "github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestAPI_EstimateGas_Deploy_OK(t *testing.T) {
	tree := abstate.NewEmptyState()
	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 3},
		gasUnitPrice:     big.NewInt(evm.DefaultGasPrice),
		blockGasLimit:    100000000000,
		log:              logger.New(t),
	}
	call := &CallEVMRequest{
		From:  test.RandomBytes(20),
		To:    nil,
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   600000,
	}
	callReq, err := cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &EstimateGasResponse{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))

	require.EqualValues(t, 148629, resp.GasUsed)
}

func TestAPI_EstimateGas_Call_OK(t *testing.T) {
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(1),
		blockGasLimit:    100000000000,
		log:              logger.New(t),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	call := &CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["get"].ID,
		Value: big.NewInt(0),
		Gas:   100000000000,
	}
	callReq, err := cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &EstimateGasResponse{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.EqualValues(t, 23479, resp.GasUsed)

	call = &CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["increment"].ID,
		Value: big.NewInt(0),
		// no gas, then by default block gas is used as max
	}
	callReq, err = cbor.Marshal(call)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder = httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp = &EstimateGasResponse{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.EqualValues(t, 45013, resp.GasUsed)
}

func TestAPI_EstimateGas_ErrorNotEnoughGas(t *testing.T) {
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(1),
		blockGasLimit:    evm.DefaultBlockGasLimit,
		log:              logger.New(t),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	call := &CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["get"].ID,
		Value: big.NewInt(0),
		Gas:   22000,
	}
	callReq, err := cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Contains(t, resp.Err, "gas required exceeds allowance")
}

func TestAPI_EstimateGas_ErrorInvalidSCParameter(t *testing.T) {
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(1),
		blockGasLimit:    evm.DefaultBlockGasLimit,
		log:              logger.New(t),
	}
	call := &CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  []byte{0, 0, 0, 0, 1, 2, 3},
		Value: big.NewInt(0),
	}
	callReq, err := cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Contains(t, resp.Err, "execution reverted")
}

func TestAPI_EstimateGas_InvalidRequest(t *testing.T) {
	tree := abstate.NewEmptyState()
	initState(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(1),
		blockGasLimit:    100000000000,
		log:              logger.New(t),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader([]byte{32}))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Equal(t, "unable to decode request body: cbor: cannot unmarshal negative integer into Go value of type api.CallEVMRequest", resp.Err)
}
