package api

import (
	"bytes"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
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

const counterContractCode = "60806040526000805534801561001457600080fd5b506101b1806100246000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80636d4ce63c1461003b578063d09de08a14610059575b600080fd5b610043610077565b60405161005091906100e9565b60405180910390f35b610061610080565b60405161006e91906100e9565b60405180910390f35b60008054905090565b600080600081548092919061009490610133565b91905055506000547f51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a8160405160405180910390a2600054905090565b6000819050919050565b6100e3816100d0565b82525050565b60006020820190506100fe60008301846100da565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061013e826100d0565b91507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff82036101705761016f610104565b5b60018201905091905056fea2646970667358221220e77ebad0c44e3c4060e53c55352c0cc28d52a30710a3437aa1345775714eeb1f64736f6c63430008120033"
const counterABI = "[\n\t{\n\t\t\"anonymous\": false,\n\t\t\"inputs\": [\n\t\t\t{\n\t\t\t\t\"indexed\": true,\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"newValue\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"name\": \"Increment\",\n\t\t\"type\": \"event\"\n\t},\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"get\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"view\",\n\t\t\"type\": \"function\"\n\t},\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"increment\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"nonpayable\",\n\t\t\"type\": \"function\"\n\t}\n]"

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

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), logger.NOP(), a).Handler.ServeHTTP(recorder, req)
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
	log := logger.New(t)
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

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), log, a).Handler.ServeHTTP(recorder, req)
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

	_, _, err = tree.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, tree.Commit())

	req = httptest.NewRequest(http.MethodPost, "/api/v1/evm/call", bytes.NewReader(callReq))
	recorder = httptest.NewRecorder()

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), log, a).Handler.ServeHTTP(recorder, req)
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

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), logger.NOP(), a).Handler.ServeHTTP(recorder, req)
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

	rpc.NewRESTServer("", 2000, observability.NOPMetrics(), logger.NOP(), a).Handler.ServeHTTP(recorder, req)
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
	require.NoError(t, s.Commit())
	return address, unit.AddressFromUnitID(details.ContractUnitID)
}
