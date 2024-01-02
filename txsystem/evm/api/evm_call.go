package api

import (
	"fmt"
	"math/big"
	"net/http"

	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/evm/unit"
	fc "github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/fxamacker/cbor/v2"
)

type CallEVMRequest struct {
	_     struct{} `cbor:",toarray"`
	From  []byte
	To    []byte
	Data  []byte
	Value *big.Int
	Gas   uint64
}

type CallEVMResponse struct {
	_                 struct{} `cbor:",toarray"`
	ProcessingDetails *evm.ProcessingDetails
}

func (a *API) CallEVM(w http.ResponseWriter, r *http.Request) {
	request := &CallEVMRequest{}
	if err := cbor.NewDecoder(r.Body).Decode(request); err != nil {
		rpc.WriteCBORError(w, fmt.Errorf("unable to decode request body: %w", err), http.StatusBadRequest, a.log)
		return
	}

	clonedState := a.state.Clone()
	defer clonedState.Revert()

	attr := &evm.TxAttributes{
		From:  request.From,
		To:    request.To,
		Data:  request.Data,
		Value: request.Value,
		Gas:   request.Gas,
	}
	res, err := a.callContract(clonedState, attr)
	if err != nil {
		rpc.WriteCBORError(w, err, http.StatusBadRequest, a.log)
		return
	}
	processingDetails := &evm.ProcessingDetails{
		ReturnData: res.ReturnData,
	}
	if res.Unwrap() != nil {
		processingDetails.ErrorDetails = fmt.Sprintf("evm runtime error: %v", res.Unwrap().Error())
	}
	// The contract address can be derived from the transaction itself
	if attr.ToAddr() == nil {
		processingDetails.ContractUnitID = unit.NewEvmAccountIDFromAddress(ethcrypto.CreateAddress(attr.FromAddr(), attr.Nonce))
	}
	stateDB := statedb.NewStateDB(clonedState, a.log)
	processingDetails.Logs = stateDB.GetLogs()

	rpc.WriteCBORResponse(w, &CallEVMResponse{ProcessingDetails: processingDetails}, http.StatusOK, a.log)
}

func (a *API) callContract(clonedState *state.State, call *evm.TxAttributes) (*core.ExecutionResult, error) {
	blockNumber := clonedState.CommittedUC().GetRoundNumber()
	stateDB := statedb.NewStateDB(clonedState, a.log)
	gp := new(core.GasPool).AddGas(math.MaxUint64)
	// Ensure message is initialized properly.
	if call.Gas == 0 {
		call.Gas = 50000000
	}
	if call.Value == nil {
		call.Value = new(big.Int)
	}

	// Set infinite balance to the fake caller account.
	u, _ := clonedState.GetUnit(unit.NewEvmAccountIDFromAddress(call.FromAddr()), false)
	var err error
	if u == nil {
		unitID := unit.NewEvmAccountIDFromAddress(call.FromAddr())
		obj := unit.NewEvmFcr(math.MaxUint64, nil, 0)
		err = clonedState.Apply(fc.AddCredit(unitID, templates.AlwaysFalseBytes(), obj))
	} else {
		unitID := unit.NewEvmAccountIDFromAddress(call.FromAddr())
		err = clonedState.Apply(fc.IncrCredit(unitID, math.MaxUint64, 0, nil))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to set fake balance %w", err)
	}
	// Execute the call.
	msg := &core.Message{
		From:              call.FromAddr(),
		To:                call.ToAddr(),
		Value:             call.Value,
		GasLimit:          call.Gas,
		GasPrice:          a.gasUnitPrice,
		GasFeeCap:         a.gasUnitPrice,
		GasTipCap:         big.NewInt(0),
		Data:              call.Data,
		AccessList:        ethtypes.AccessList{},
		SkipAccountChecks: true,
	}
	blockCtx := evm.NewBlockContext(blockNumber, memorydb.New())
	simEvm := vm.NewEVM(blockCtx, evm.NewTxContext(call, a.gasUnitPrice), stateDB, evm.NewChainConfig(new(big.Int).SetBytes(a.systemIdentifier)), evm.NewVMConfig())
	// Apply the transaction to the current state (included in the env)
	return core.ApplyMessage(simEvm, msg, gp)
}
