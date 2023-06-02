package abruntime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/wvm"
)

// ValidateResult interpret program return value. Currently only return code is expected, but
// this should be most likely be based on the application and hence should be moved to alphabill specific runtime
func validateResult(retVal []uint64) error {
	if len(retVal) > 1 {
		return fmt.Errorf("unexpected return value length %v", len(retVal))
	}
	// todo: translate error code to go error
	if retVal[0] != wvm.Success {
		return fmt.Errorf("program exited with error %v", retVal[0])
	}
	return nil
}

// Call set-up wasm VM and call function
func Call(ctx context.Context, wasm []byte, fName string, execCtx wvm.ExecutionCtx, storage wvm.Storage) (err error) {
	// todo: AB-1006 automatic revert of changes when program execution fails,
	abHostMod, err := wvm.BuildABHostModule(execCtx, storage)
	if err != nil {
		return fmt.Errorf("failed to intialize ab host module, %w", err)
	}

	vm, err := wvm.New(ctx, wasm, wvm.WithHostModule(abHostMod))
	if err != nil {
		return fmt.Errorf("wasm program load failed, %w", err)
	}
	defer func() { err = errors.Join(err, vm.Close(ctx)) }()
	// check API
	fn, err := vm.GetApiFn(fName)
	if err != nil {
		return fmt.Errorf("program call failed, %w", err)
	}
	// copy input data
	var result []uint64
	// API calls have no parameters, there is a host callback to get input parameters
	// all programs must complete in 100 ms, this will later be replaced with gas cost
	// for now just set a hard limit to make sure programs do not run forever
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	result, err = fn.Call(ctx)
	if err != nil {
		return fmt.Errorf("program call failed, %w", err)
	}
	if err = validateResult(result); err != nil {
		return fmt.Errorf("program call exited with error, %w", err)
	}
	return nil
}

func CheckProgram(ctx context.Context, wasm []byte, execCtx wvm.ExecutionCtx) error {
	// check wasm source, init wasm VM
	abHostMod, err := wvm.BuildABHostModule(execCtx, wvm.NewMemoryStorage())
	if err != nil {
		return fmt.Errorf("failed to intialize ab host module, %w", err)
	}
	vm, err := wvm.New(ctx, wasm, wvm.WithHostModule(abHostMod))
	if err != nil {
		return fmt.Errorf("wasm vm init error, %w", err)
	}
	// check API
	if err = vm.CheckApiCallExists(); err != nil {
		return fmt.Errorf("wasm program error, %w", err)
	}
	return nil
}
