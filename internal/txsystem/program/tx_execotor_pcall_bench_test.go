package program

import (
	"context"
	"crypto"
	"encoding/binary"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"google.golang.org/protobuf/types/known/anypb"
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

func newPCallTx(id []byte, sysID []byte, fName string) *PCallTransactionOrder {
	order := &txsystem.Transaction{
		SystemId:              sysID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                id,
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: 10, MaxFee: 2},
		ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
	}
	return &PCallTransactionOrder{
		txOrder: order,
		attributes: &PCallAttributes{
			Function: fName,
			Input:    uint64ToLEBytes(1),
		},
	}
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
	pCallOrder := newPCallTx(counterProgramUnitID.Bytes(), systemID, "count")
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := handlePCallTx(ctx, state, systemID, crypto.SHA256)(pCallOrder, 10000); err != nil {
					b.Fatal("program call transaction failed, %w", err)
				}
			}
		})
	}
}
