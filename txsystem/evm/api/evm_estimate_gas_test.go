package api

import (
	"bytes"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/rpc"
	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestAPI_EstimateGas_Deploy_OK(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	a := &API{
		state:               tree,
		partitionID:         3,
		gasUnitPrice:        big.NewInt(evm.DefaultGasPrice),
		blockGasLimit:       100000000000,
		log:                 observe.Logger(),
	}
	call := &evmsdk.CallEVMRequest{
		From:  test.RandomBytes(20),
		To:    nil,
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   600000,
	}
	callReq, err := types.Cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &EstimateGasResponse{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))

	require.EqualValues(t, 177717, resp.GasUsed)
}

func TestAPI_EstimateGas_Call_OK(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:               tree,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(1),
		blockGasLimit:       100000000000,
		log:                 observe.Logger(),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	call := &evmsdk.CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["get"].ID,
		Value: big.NewInt(0),
		Gas:   100000000000,
	}
	callReq, err := types.Cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &EstimateGasResponse{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.EqualValues(t, 23377, resp.GasUsed)

	call = &evmsdk.CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["increment"].ID,
		Value: big.NewInt(0),
		// no gas, then by default block gas is used as max
	}
	callReq, err = types.Cbor.Marshal(call)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder = httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp = &EstimateGasResponse{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.EqualValues(t, 44813, resp.GasUsed)
}

func TestAPI_EstimateGas_ErrorNotEnoughGas(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:               tree,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(1),
		blockGasLimit:       evm.DefaultBlockGasLimit,
		log:                 observe.Logger(),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	call := &evmsdk.CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["get"].ID,
		Value: big.NewInt(0),
		Gas:   22000,
	}
	callReq, err := types.Cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Contains(t, resp.Err, "gas required exceeds allowance")
}

func TestAPI_EstimateGas_ErrorIntrinsicGas(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:               tree,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(1),
		blockGasLimit:       evm.DefaultBlockGasLimit,
		log:                 observe.Logger(),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	call := &evmsdk.CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["get"].ID,
		Value: big.NewInt(0),
		Gas:   21000,
	}
	callReq, err := types.Cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Contains(t, resp.Err, "gas required exceeds allowance")
}

func TestAPI_EstimateGas_ErrorRevert(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:               tree,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(1),
		blockGasLimit:       evm.DefaultBlockGasLimit,
		log:                 observe.Logger(),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)
	// try to reset to 2, will result in revert
	fnCall, err := cABI.Pack("reset", big.NewInt(2))
	require.NoError(t, err)
	call := &evmsdk.CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  fnCall,
		Value: big.NewInt(0),
	}
	callReq, err := types.Cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, resp.Err, "execution reverted: can only be reset to 0")
}

func TestAPI_EstimateGas_CallInfinite(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:               tree,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(1),
		blockGasLimit:       evm.DefaultBlockGasLimit,
		log:                 observe.Logger(),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)
	require.NoError(t, err)
	call := &evmsdk.CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["infiniteInc"].ID,
		Gas:   50000,
		Value: big.NewInt(0),
	}
	callReq, err := types.Cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, resp.Err, "out of gas: not enough gas for reentrancy sentry")
}

func TestAPI_EstimateGas_ErrorInvalidSCParameter(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:               tree,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(1),
		blockGasLimit:       evm.DefaultBlockGasLimit,
		log:                 observe.Logger(),
	}
	call := &evmsdk.CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  []byte{0, 0, 0, 0, 1, 2, 3},
		Value: big.NewInt(0),
	}
	callReq, err := types.Cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()
	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Contains(t, resp.Err, "execution reverted")
}

func TestAPI_EstimateGas_InvalidRequest(t *testing.T) {
	observe := observability.Default(t)
	tree := abstate.NewEmptyState()
	initState(t, tree)

	a := &API{
		state:               tree,
		partitionID:         1,
		gasUnitPrice:        big.NewInt(1),
		blockGasLimit:       100000000000,
		log:                 observe.Logger(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/estimateGas", bytes.NewReader([]byte{32}))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, types.Cbor.Decode(recorder.Body, resp))
	require.Equal(t, "unable to decode request body: cbor: cannot unmarshal negative integer into Go value of type evm.CallEVMRequest", resp.Err)
}
