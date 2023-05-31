package wasmruntime

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/wvm"
)

// Call set-up wasm VM and call function
func Call(ctx context.Context, wasm []byte, fName string, execCtx wvm.ExecutionCtx, storage wvm.Storage) error {
	// todo: AB-1006 automatic revert of changes when program execution fails,
	vm, err := wvm.New(ctx, wasm, execCtx, wvm.WithStorage(storage))
	if err != nil {
		return fmt.Errorf("wasm program load failed, %w", err)
	}
	// check API
	fn, err := vm.GetApiFn(fName)
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
	if err = wvm.ValidateResult(result); err != nil {
		return fmt.Errorf("program call exited with error, %w", err)
	}
	return nil
}

func CheckProgram(ctx context.Context, wasm []byte, execCtx wvm.ExecutionCtx) error {
	// check wasm source, init wasm VM
	vm, err := wvm.New(ctx, wasm, execCtx)
	if err != nil {
		return fmt.Errorf("wasm vm init error, %w", err)
	}
	// check API
	if err = vm.CheckApiCallExists(); err != nil {
		return fmt.Errorf("wasm program error, %w", err)
	}
	return nil
}
