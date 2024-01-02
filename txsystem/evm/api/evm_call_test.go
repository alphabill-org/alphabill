package api

import (
	"bytes"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	teststate "github.com/alphabill-org/alphabill/internal/testutils/state"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc"
	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/evm/unit"
	fc "github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

var systemIdentifier = []byte{0, 0, 4, 2}

// SPDX-License-Identifier: GPL-3.0
/*
pragma solidity >=0.4.0 <0.9.0;

contract Counter {

uint256 value=0;

event Increment(
uint256 indexed newValue
);

event Reset();

function reset(uint256 resValue) public {
if (resValue != 0)
revert("can only be reset to 0");
value = 0;
emit Reset();
}

function increment() public returns(uint256) {
value++;
emit Increment(value);
return value;
}

function infiniteInc() public returns(uint256) {
uint256 i = 1;
while(i == 1){
value = value + 1;
}
return value;
}

function get() public view returns (uint256) {
return value;
}
}
*/
const counterContractCode = "60806040526000805534801561001457600080fd5b50610238806100246000396000f3fe608060405234801561001057600080fd5b506004361061004c5760003560e01c8063310bd74b146100515780636d4ce63c1461007f578063a1edba8d1461009d578063d09de08a146100bb575b600080fd5b61007d6004803603602081101561006757600080fd5b81019080803590602001909291905050506100d9565b005b610087610185565b6040518082815260200191505060405180910390f35b6100a561018e565b6040518082815260200191505060405180910390f35b6100c36101b9565b6040518082815260200191505060405180910390f35b6000811461014f576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260168152602001807f63616e206f6e6c7920626520726573657420746f20300000000000000000000081525060200191505060405180910390fd5b600080819055507f6423db340205c829eeb91151b1c5d1dc6d7a2b8708b1621494e89ba90c87081e60405160405180910390a150565b60008054905090565b600080600190505b60018114156101b057600160005401600081905550610196565b60005491505090565b600080600081548092919060010191905055506000547f51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a8160405160405180910390a260005490509056fea265627a7a72315820d9b1abe750615f1bda84372d000bc03d194fbcfb1d353b001a541cdc2d97a28564736f6c63430005100032"
const counterABI = "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"newValue\",\"type\":\"uint256\"}],\"name\":\"Increment\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[],\"name\":\"Reset\",\"type\":\"event\"},{\"constant\":true,\"inputs\":[],\"name\":\"get\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"increment\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"infiniteInc\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"resValue\",\"type\":\"uint256\"}],\"name\":\"reset\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]"

func TestAPI_CallEVM_CleanState_OK(t *testing.T) {
	tree := abstate.NewEmptyState()
	treeClean := tree.Clone()
	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 3},
		gasUnitPrice:     big.NewInt(evm.DefaultGasPrice),
		blockGasLimit:    evm.DefaultBlockGasLimit,
		log:              logger.New(t),
	}
	call := &CallEVMRequest{
		From: test.RandomBytes(20),
		Data: common.Hex2Bytes(counterContractCode),
		Gas:  600000,
	}
	callReq, err := cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/call", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &CallEVMResponse{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))

	require.Empty(t, resp.ProcessingDetails.ErrorDetails)
	require.NotEmpty(t, resp.ProcessingDetails.ContractUnitID)
	require.NotEmpty(t, resp.ProcessingDetails.ReturnData, common.Address{})
	require.Empty(t, resp.ProcessingDetails.Logs)
	// make sure state is reverted back
	require.Equal(t, treeClean, tree)
}

func TestAPI_CallEVM_OK(t *testing.T) {
	observe := observability.Default(t)
	log := observe.Logger()
	tree := abstate.NewEmptyState()
	address, contractAddr := initState(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(1),
		blockGasLimit:    evm.DefaultBlockGasLimit,
		log:              log,
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)
	call := &CallEVMRequest{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["get"].ID,
		Value: big.NewInt(0),
		Gas:   29000,
	}
	callReq, err := cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/call", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &CallEVMResponse{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))

	require.Equal(t, make([]byte, 32), resp.ProcessingDetails.ReturnData)

	callContract := &evm.TxAttributes{
		From:  address.Bytes(),
		To:    contractAddr.Bytes(),
		Data:  cABI.Methods["increment"].ID,
		Gas:   53000,
		Value: big.NewInt(0),
		Nonce: 1,
	}
	gasPool := new(core.GasPool).AddGas(math.MaxUint64)
	gasPrice := big.NewInt(evm.DefaultGasPrice)
	_, err = evm.Execute(1, statedb.NewStateDB(tree, log), memorydb.New(), callContract, systemIdentifier, gasPool, gasPrice, false, log)
	require.NoError(t, err)
	teststate.CommitWithUC(t, tree)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/evm/call", bytes.NewReader(callReq))
	recorder = httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observe, a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp = &CallEVMResponse{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Equal(t, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1}, resp.ProcessingDetails.ReturnData)
}

func TestAPI_CallEVM_ToFieldMissing(t *testing.T) {
	tree := abstate.NewEmptyState()
	address, _ := initState(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(evm.DefaultGasPrice),
		blockGasLimit:    evm.DefaultBlockGasLimit,
		log:              logger.New(t),
	}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)
	call := &CallEVMRequest{
		From: address.Bytes(),
		// To: is missing, should be a deployed contract address
		Data: cABI.Methods["get"].ID,
	}
	callReq, err := cbor.Marshal(call)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/call", bytes.NewReader(callReq))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	// this is an ok call, no an error, but You have to pay for your nonsense
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := &CallEVMResponse{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Empty(t, resp.ProcessingDetails.ErrorDetails)
	require.NotEmpty(t, resp.ProcessingDetails.ContractUnitID)
	require.Empty(t, resp.ProcessingDetails.ReturnData, common.Address{})
	require.Empty(t, resp.ProcessingDetails.Logs)
}

func TestAPI_CallEVM_InvalidRequest(t *testing.T) {
	tree := abstate.NewEmptyState()
	initState(t, tree)

	a := &API{
		state:            tree,
		systemIdentifier: []byte{0, 0, 0, 1},
		gasUnitPrice:     big.NewInt(evm.DefaultGasPrice),
		blockGasLimit:    evm.DefaultBlockGasLimit,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evm/call", bytes.NewReader([]byte{32}))
	recorder := httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPObservability(), a).Handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := &struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{}
	require.NoError(t, cbor.NewDecoder(recorder.Body).Decode(resp))
	require.Equal(t, "unable to decode request body: cbor: cannot unmarshal negative integer into Go value of type api.CallEVMRequest", resp.Err)
}

func initState(t *testing.T, s *abstate.State) (common.Address, common.Address) {
	log := logger.New(t)
	// simulate added fee credit
	address := common.BytesToAddress(test.RandomBytes(20))
	unitID := unit.NewEvmAccountIDFromAddress(address)
	var balance uint64 = 500000
	backlink := test.RandomBytes(32)
	err := s.Apply(fc.AddCredit(unitID, templates.AlwaysTrueBytes(), unit.NewEvmFcr(balance, backlink, 10)))
	require.NoError(t, err)

	stateDB := statedb.NewStateDB(s, log)

	evmAttr := &evm.TxAttributes{
		From:  address.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   600000,
		Nonce: 0,
	}
	gasPool := new(core.GasPool).AddGas(math.MaxUint64)
	gasPrice := big.NewInt(evm.DefaultGasPrice)
	sm, err := evm.Execute(1, stateDB, memorydb.New(), evmAttr, systemIdentifier, gasPool, gasPrice, false, log)
	details := &evm.ProcessingDetails{}
	require.NoError(t, err)
	require.NoError(t, sm.UnmarshalDetails(details))
	_, _, err = s.CalculateRoot()
	require.NoError(t, err)
	teststate.CommitWithUC(t, s)
	return address, unit.AddressFromUnitID(details.ContractUnitID)
}
