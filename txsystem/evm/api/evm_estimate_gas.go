package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

type EstimateGasResponse struct {
	_       struct{} `cbor:",toarray"`
	GasUsed uint64
}

func (a *API) EstimateGas(w http.ResponseWriter, r *http.Request) {
	request := &CallEVMRequest{}
	if err := types.Cbor.Decode(r.Body, request); err != nil {
		rpc.WriteCBORError(w, fmt.Errorf("unable to decode request body: %w", err), http.StatusBadRequest, a.log)
		return
	}
	callAttr := &evm.TxAttributes{
		From:  request.From,
		To:    request.To,
		Data:  request.Data,
		Value: request.Value,
		Gas:   request.Gas,
	}
	// Determine the lowest and highest possible gas limits to binary search in between
	var (
		lo  uint64 = params.TxGas - 1
		hi  uint64
		cap uint64
	)
	if callAttr.Gas >= params.TxGas {
		hi = callAttr.Gas
	} else {
		hi = a.blockGasLimit
	}
	cap = hi

	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(gas uint64) (bool, *core.ExecutionResult, error) {
		callAttr.Gas = gas

		clonedState := a.state.Clone()
		defer clonedState.Revert()
		res, err := a.callContract(clonedState, callAttr)

		if err != nil {
			if errors.Is(err, core.ErrIntrinsicGas) {
				return true, nil, nil // Special case, raise gas limit
			}
			return true, nil, err // Bail out
		}
		return res.Failed(), res, nil
	}
	// Execute the binary search and hone in on an executable gas limit
	for lo+1 < hi {
		mid := (hi + lo) / 2
		failed, _, err := executable(mid)

		if err != nil {
			// If the error is not nil(consensus error), it means the provided message
			// call or transaction will never be accepted no matter how much gas it is
			// assigned.
			// Return the error and that is it.
			rpc.WriteCBORError(w, err, http.StatusBadRequest, a.log)
			return
		}
		if failed {
			lo = mid
		} else {
			hi = mid
		}
	}
	// Reject the transaction as invalid if it still fails at the highest allowance
	if hi == cap {
		failed, result, err := executable(hi)
		if err != nil {
			rpc.WriteCBORError(w, err, http.StatusBadRequest, a.log)
			return
		}
		if failed {
			if result != nil && result.Err != vm.ErrOutOfGas {
				if len(result.Revert()) > 0 {
					rpc.WriteCBORError(w, newRevertError(result), http.StatusBadRequest, a.log)
					return
				}
				rpc.WriteCBORError(w, result.Err, http.StatusBadRequest, a.log)
				return
			}
			// Otherwise, the specified gas cap is too low
			rpc.WriteCBORError(w, fmt.Errorf("gas required exceeds allowance (%d)", cap), http.StatusBadRequest, a.log)
			return
		}
	}
	rpc.WriteCBORResponse(w, &EstimateGasResponse{GasUsed: hi}, http.StatusOK, a.log)
}

func newRevertError(result *core.ExecutionResult) error {
	reason, errUnpack := abi.UnpackRevert(result.Revert())
	if errUnpack == nil {
		return fmt.Errorf("execution reverted: %v", reason)
	}
	return fmt.Errorf("execution reverted: %v", hexutil.Encode(result.Revert()))
}
