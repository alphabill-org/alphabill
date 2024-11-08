package api

import (
	"fmt"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
)

func (a *API) CallEVM(w http.ResponseWriter, r *http.Request) {
	request := &evmsdk.CallEVMRequest{}
	if err := types.Cbor.Decode(r.Body, request); err != nil {
		WriteCBORError(w, fmt.Errorf("unable to decode request body: %w", err), http.StatusBadRequest, a.log)
		return
	}

	clonedState := a.state.Clone()
	defer clonedState.Revert()

	attr := &evmsdk.TxAttributes{
		From:  request.From,
		To:    request.To,
		Data:  request.Data,
		Value: request.Value,
		Gas:   request.Gas,
	}
	res, err := a.callContract(clonedState, attr)
	if err != nil {
		WriteCBORError(w, err, http.StatusBadRequest, a.log)
		return
	}
	processingDetails := &evmsdk.ProcessingDetails{
		ReturnData: res.ReturnData,
	}
	if res.Unwrap() != nil {
		processingDetails.ErrorDetails = fmt.Sprintf("evm runtime error: %v", res.Unwrap().Error())
	}
	// The contract address can be derived from the transaction itself
	if attr.ToAddr() == nil {
		// Deriving the signer is expensive, only do if it's actually needed
		processingDetails.ContractAddr = ethcrypto.CreateAddress(attr.FromAddr(), attr.Nonce)
	}
	stateDB := statedb.NewStateDB(clonedState, a.log)
	processingDetails.Logs = stateDB.GetLogs()

	WriteCBORResponse(w, &evmsdk.CallEVMResponse{ProcessingDetails: processingDetails}, http.StatusOK, a.log)
}

func (a *API) callContract(clonedState *state.State, call *evmsdk.TxAttributes) (*core.ExecutionResult, error) {
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
	u, _ := clonedState.GetUnit(call.From, false)
	var err error
	balance := uint256.MustFromBig(math.MaxBig256)
	if u == nil {
		err = clonedState.Apply(statedb.CreateAccountAndAddCredit(call.FromAddr(), templates.AlwaysFalseBytes(), balance, 0))
	} else {
		err = clonedState.Apply(statedb.SetBalance(call.From, balance))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to set fake balance: %w", err)
	}
	// Execute the call.
	msg := &core.Message{
		From:             call.FromAddr(),
		To:               call.ToAddr(),
		Value:            call.Value,
		GasLimit:         call.Gas,
		GasPrice:         a.gasUnitPrice,
		GasFeeCap:        a.gasUnitPrice,
		GasTipCap:        big.NewInt(0),
		Data:             call.Data,
		AccessList:       ethtypes.AccessList{},
		SkipNonceChecks:  true,
		SkipFromEOACheck: true,
	}
	db, err := memorydb.New()
	if err != nil {
		return nil, fmt.Errorf("creating block DB: %w", err)
	}
	blockCtx := evm.NewBlockContext(blockNumber, db)
	simEvm := vm.NewEVM(blockCtx, evm.NewTxContext(call, a.gasUnitPrice), stateDB, evm.NewChainConfig(new(big.Int).SetBytes(a.partitionIdentifier.Bytes())), evm.NewVMConfig())
	// Apply the transaction to the current state (included in the env)
	return core.ApplyMessage(simEvm, msg, gp)
}
