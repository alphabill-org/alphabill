package program

import (
	"context"
	"crypto"
	"encoding/binary"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

func deployCounterProgram(b *testing.B) *rma.Tree {
	state := rma.NewWithSHA256()
	counterStateId := CreateStateFileID(counterProgramUnitID, util.Uint32ToBytes(counterState))
	cnt := make([]byte, 8)
	binary.LittleEndian.PutUint64(cnt, 1)
	// add both state and program
	if err := state.AtomicUpdate(
		rma.AddItem(counterProgramUnitID,
			script.PredicateAlwaysFalse(),
			&Program{wasm: counterWasm, progParams: []byte{1, 0, 0, 0, 0, 0, 0, 0}},
			make([]byte, 32)),
		rma.AddItem(counterStateId,
			script.PredicateAlwaysFalse(),
			&StateFile{bytes: cnt},
			make([]byte, 32))); err != nil {
		b.Fatal("could not deploy counter program")
	}
	state.Commit()
	return state
}

func newPCallTx(id []byte, sysID []byte, fName string) (*types.TransactionOrder, *PCallAttributes) {
	attr := &PCallAttributes{
		FuncName:  fName,
		InputData: []byte{1, 0, 0, 0, 0, 0, 0, 0},
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       sysID,
			Type:           ProgramCall,
			UnitID:         id,
			ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
		},
	}
	return tx, attr
}

func Benchmark(b *testing.B) {
	ctx := context.Background()
	benchmarks := []struct {
		name      string
		nofPCalls uint64
	}{
		{
			name: "program call benchmark",
		},
	}
	state := deployCounterProgram(b)
	tx, attr := newPCallTx(counterProgramUnitID.Bytes(), systemID, "count")
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := handlePCallTx(ctx, state, systemID, crypto.SHA256)(tx, attr, 10000); err != nil {
					b.Fatal("program call transaction failed, %w", err)
				}
			}
		})
	}
}
