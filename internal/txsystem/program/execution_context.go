package program

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type ExecutionContext struct {
	hashAlgorithm crypto.Hash
	programID     *uint256.Int
	wasm          []byte
	inputParams   []byte
	progParams    []byte
	txHash        []byte
}

func NewExecCtxFormPCall(txOrder *PCallTransactionOrder, state *rma.Tree, hash crypto.Hash) (*ExecutionContext, error) {
	id := txOrder.UnitID()
	unit, err := state.GetUnit(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load program '%X': %w", util.Uint256ToBytes(id), err)
	}
	prog, ok := unit.Data.(*Program)
	if !ok {
		return nil, fmt.Errorf("unit %X does not contain program data", util.Uint256ToBytes(id))
	}
	if len(prog.Wasm()) < 1 {
		return nil, fmt.Errorf("unit %X does not contain wasm code", util.Uint256ToBytes(id))
	}
	return &ExecutionContext{
		hashAlgorithm: hash,
		programID:     id,
		wasm:          prog.Wasm(),
		inputParams:   txOrder.attributes.Input,
		progParams:    prog.ProgParams(),
		txHash:        txOrder.Hash(hash),
	}, nil
}

func NewExecCtxFormDeploy(txOrder *PDeployTransactionOrder, state *rma.Tree, hash crypto.Hash) (*ExecutionContext, error) {
	id := txOrder.UnitID()
	if _, err := state.GetUnit(id); err == nil {
		return nil, fmt.Errorf("program unit with id '%X' already exists", util.Uint256ToBytes(id))
	}
	if len(txOrder.attributes.Program) < 1 {
		return nil, fmt.Errorf("unit %X does not contain wasm code", util.Uint256ToBytes(id))
	}
	return &ExecutionContext{
		hashAlgorithm: hash,
		programID:     id,
		wasm:          txOrder.attributes.Program,
		inputParams:   txOrder.attributes.InitData,
		progParams:    txOrder.attributes.InitData,
		txHash:        txOrder.Hash(hash),
	}, nil
}

func (e ExecutionContext) GetProgramID() *uint256.Int {
	return e.programID
}

func (e ExecutionContext) Wasm() []byte {
	return e.wasm
}

func (e ExecutionContext) GetInputData() []byte {
	return e.inputParams
}

func (e ExecutionContext) GetParams() []byte {
	return e.progParams
}

func (e ExecutionContext) GetTxHash() []byte {
	return e.txHash
}
