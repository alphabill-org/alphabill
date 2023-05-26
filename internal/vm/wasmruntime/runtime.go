package wasmruntime

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/vm"
)

// Call set-up wasm VM and call function
func Call(ctx context.Context, execCtx vm.ExecutionCtx, fName string, storage vm.Storage) error {
	// todo: AB-1006 automatic revert of changes when program execution fails,
	wvm, err := vm.New(ctx, execCtx, vm.WithStorage(storage))
	if err != nil {
		return fmt.Errorf("wasm program load failed, %w", err)
	}
	// check API
	fn, err := wvm.GetApiFn(fName)
	if err != nil {
		return fmt.Errorf("program call failed, %w", err)
	}
	// copy input data
	var result []uint64
	// API calls have no parameters, there is a host callback to get input parameters
	result, err = fn.Call(ctx)
	if err != nil {
		return fmt.Errorf("program call failed, %w", err)
	}
	if err = vm.ValidateResult(result); err != nil {
		return fmt.Errorf("program call exited with error, %w", err)
	}
	return nil
}

func CheckProgram(ctx context.Context, execCtx vm.ExecutionCtx) error {
	// check wasm source, init wasm VM
	wvm, err := vm.New(ctx, execCtx)
	if err != nil {
		return fmt.Errorf("wasm program load failed, %w", err)
	}
	// check API
	if err = wvm.CheckApiCallExists(); err != nil {
		return fmt.Errorf("wasm program error, %w", err)
	}
	return nil
}
