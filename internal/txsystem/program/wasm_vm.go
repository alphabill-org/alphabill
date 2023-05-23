package program

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// todo: make optional
const MaxMemoryPages = 65536

type WasmVM struct {
	runtime wazero.Runtime
	mod     api.Module
}

func Call(ctx context.Context, id *uint256.Int, fName string, input []byte, state *rma.Tree, txHash []byte) error {
	unit, err := state.GetUnit(id)
	if err != nil {
		return fmt.Errorf("failed to load program '%X': %w", util.Uint256ToBytes(id), err)
	}
	src, ok := unit.Data.(*Program)
	if !ok {
		return fmt.Errorf("unit %X does not contain program data", util.Uint256ToBytes(id))
	}
	if len(src.Wasm()) < 1 {
		return fmt.Errorf("unit %X does not contain wasm code", util.Uint256ToBytes(id))
	}
	batchUpdate, err := state.StartBatchUpdate()
	defer batchUpdate.Revert()
	if err != nil {
		return fmt.Errorf("failed to init state batch update, %w", err)
	}

	vm, err := NewVM(ctx, id, src.Wasm(), batchUpdate, txHash)
	if err != nil {
		return fmt.Errorf("wasm vm init failed, %w", err)
	}
	// check API
	fn, err := vm.GetApiFn(fName)
	if err != nil {
		return fmt.Errorf("pcall tx failed, %w", err)
	}
	// copy input data
	var result []uint64
	result, err = fn.Call(ctx, 0, 0)
	if err != nil {
		return fmt.Errorf("scall tx execute failed, %w", err)
	}
	if err = validateResult(result); err != nil {
		return fmt.Errorf("scall tx execute failed, %w", err)
	}
	// commit any changes made to program state files (deferred revert will become noop)
	batchUpdate.Commit()
	// check and todo: call init
	return nil
}

func ExecuteAndDeploy(ctx context.Context, id *uint256.Int, code []byte, data []byte, state *rma.Tree, txHash []byte) error {
	batchUpdate, err := state.StartBatchUpdate()
	defer batchUpdate.Revert()
	if err != nil {
		return fmt.Errorf("failed to init state batch update, %w", err)
	}
	vm, err := NewVM(ctx, id, code, batchUpdate, txHash)
	if err != nil {
		return fmt.Errorf("wasm vm init failed, %w", err)
	}
	// check API
	if err = vm.CheckApiCallExists(); err != nil {
		return fmt.Errorf("pdeploy tx failed, %w", err)
	}
	if err = batchUpdate.AddItem(
		id,
		script.PredicateAlwaysFalse(),
		&Program{wasm: code, initData: data},
		make([]byte, 32)); err != nil {
		return fmt.Errorf("pcall tx failed to deplopy program, %w", err)
	}
	// commit any changes made to program state files (deferred revert will become noop)
	batchUpdate.Commit()
	return nil
}

func addHostFunctions(ctx context.Context, rt wazero.Runtime, state *rma.BatchUpdate, parentUnit *uint256.Int, txHash []byte) error {
	// Dummy test for host defined functions
	_, err := rt.NewHostModuleBuilder("ab").
		NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, fileID, offset, size uint32) int32 {
		logger.Debug("state request, %v", fileID)
		stateId := txutil.SameShardID(parentUnit, sha256.New().Sum(util.Uint32ToBytes(fileID)))
		u, err := state.GetUnit(stateId)
		if err != nil {
			logger.Warning("get state from system failed, %v", err)
			return -1
		}
		prgFile, ok := u.Data.(*StateFile)
		if !ok {
			return -1
		}
		if uint32(len(prgFile.GetBytes())) > size {
			logger.Warning("state too big")
			return -2
		}
		if ok = m.Memory().Write(offset, prgFile.GetBytes()); !ok {
			logger.Warning("state write failed")
			return -1
		}
		return int32(len(prgFile.GetBytes()))
	}).Export("get_state").
		NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, fileID, offset, cnt uint32) int32 {
		fBytes, ok := m.Memory().Read(offset, cnt)
		if !ok {
			logger.Warning("failed to read state from program memory")
			return -1
		}
		logger.Debug("set state, %v id, new state: %v", fileID, fBytes)
		stateId := txutil.SameShardID(parentUnit, sha256.New().Sum(util.Uint32ToBytes(fileID)))
		_, err := state.GetUnit(stateId)
		if errors.Is(err, rma.ErrUnitNotFound) {
			if err = state.AddItem(stateId, script.PredicateAlwaysFalse(), &StateFile{bytes: fBytes}, txHash); err != nil {
				logger.Warning("failed to persist program file")
				return -1
			}
		} else {
			updateFunc := func(data rma.UnitData) rma.UnitData {
				return &StateFile{bytes: fBytes}
			}
			if err = state.UpdateData(stateId, updateFunc, txHash); err != nil {
				logger.Warning("failed to persist program file")
				return -1
			}
		}
		return 0
	}).Export("set_state").
		NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, offset, size uint32) int32 {
		logger.Debug("get init data")
		u, err := state.GetUnit(parentUnit)
		if err != nil {
			logger.Warning("get program unit failed, %v", err)
			return -1
		}
		prg, ok := u.Data.(*Program)
		if !ok {
			logger.Warning("invalid unit type")
			return -1
		}
		if uint32(len(prg.InitData())) > size {
			logger.Warning("state too big")
			return -2
		}
		if ok = m.Memory().Write(offset, prg.InitData()); !ok {
			logger.Warning("state write failed")
			return -1
		}
		return int32(len(prg.InitData()))
	}).Export("get_init_data").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("host functions instanciate failed")
	}
	return nil
}

func NewVM(ctx context.Context, id *uint256.Int, wasmSrc []byte, state *rma.BatchUpdate, txHash []byte) (*WasmVM, error) {
	cfg := wazero.NewRuntimeConfig().WithCloseOnContextDone(true).WithMemoryLimitPages(MaxMemoryPages)
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	if err := addHostFunctions(ctx, rt, state, id, txHash); err != nil {
		return nil, fmt.Errorf("host iterface init failed, %w", err)
	}
	m, err := rt.Instantiate(ctx, wasmSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate VM with wasm source, %w", err)
	}
	return &WasmVM{
		runtime: rt,
		mod:     m,
	}, nil
}

func (vm *WasmVM) CheckApiCallExists() error {
	if len(vm.mod.ExportedFunctionDefinitions()) < 1 {
		return fmt.Errorf("no exported functions")
	}
	return nil
}

func (vm *WasmVM) GetApiFn(fnName string) (api.Function, error) {
	fn := vm.mod.ExportedFunction(fnName)
	if fn == nil {
		return nil, fmt.Errorf("function %v not found", fnName)
	}
	return fn, nil
}

func validateResult(retVal []uint64) error {
	if len(retVal) > 1 {
		return fmt.Errorf("unexpected return value length %v", len(retVal))
	}
	// todo: translate error code to go error
	if retVal[0] != 0 {
		return fmt.Errorf("program exited with error %v", retVal[0])
	}
	return nil
}
