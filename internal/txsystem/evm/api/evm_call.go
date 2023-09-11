package api

import (
	"errors"
	"fmt"
	"math/big"
	"net/http"

	"github.com/alphabill-org/alphabill/internal/txsystem/evm"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/core"
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
		util.WriteCBORError(w, fmt.Errorf("unable to decode request body: %w", err), http.StatusBadRequest)
		return
	}

	if request.To == nil {
		util.WriteCBORError(w, errors.New("to field is missing"), http.StatusBadRequest)
		return
	}
	clonedState := a.state.Clone()
	defer clonedState.Revert()

	blockNumber := clonedState.CommittedTreeBlockNumber()
	stateDB := statedb.NewStateDB(clonedState)
	gp := new(core.GasPool).AddGas(a.blockGasLimit)
	attr := &evm.TxAttributes{
		From:  request.From,
		To:    request.To,
		Data:  request.Data,
		Value: request.Value,
		Gas:   request.Gas,
	}
	result, err := evm.Execute(blockNumber, stateDB, attr, a.systemIdentifier, gp, a.gasUnitPrice, true)
	if err != nil {
		util.WriteCBORError(w, err, http.StatusBadRequest)
		return
	}

	details := &evm.ProcessingDetails{}
	if err := cbor.Unmarshal(result.ProcessingDetails, details); err != nil {
		util.WriteCBORError(w, errors.New("unable to read EVM result"), http.StatusBadRequest)
		return
	}

	util.WriteCBORResponse(w, &CallEVMResponse{ProcessingDetails: details}, http.StatusOK)
}
