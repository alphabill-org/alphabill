package program

import (
	"crypto"
	_ "embed"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

// TODO current state supports 256 bit unit identifiers (AB-416)
var counterProgramID = append(make([]byte, 24), []byte{0, 0, 0, 0, 0, 's', 'y', 's'}...)

//go:embed counter.wasm
var counterWasm []byte

type (
	BuiltInPrograms map[string]BuiltInProgram

	BuiltInProgram func(ctx *ProgramExecutionContext) error

	ProgramExecutionContext struct {
		systemIdentifier []byte
		hashAlgorithm    crypto.Hash
		state            *rma.Tree
		runtime          *WasmVM
		txOrder          *PCallTransactionOrder
	}
)

func initBuiltInPrograms(state *rma.Tree) error {
	if state == nil {
		return errors.New("state is nil")
	}
	counterId := uint256.NewInt(0).SetBytes(counterProgramID)

	if err := state.AtomicUpdate(
		rma.AddItem(
			counterId,
			script.PredicateAlwaysFalse(),
			&Program{wasm: counterWasm, initData: util.Uint64ToBytes(1)},
			make([]byte, 32)),
	); err != nil {
		return fmt.Errorf("failed to add 'unlock money bill' program to the state: %w", err)
	}
	state.Commit()
	return nil
}
