package program

import (
	"context"
	"crypto"
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const counterState uint32 = 0xaabbccdd

var systemID = []byte{0, 0, 0, 3}
var counterProgramUnitID = uint256.NewInt(0).SetBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'c', 'o', 'u', 'n', 't', 'e', 'r'})

//go:embed test_program/counter.wasm
var counterWasm []byte

func Test_handlePCallTx(t *testing.T) {
	type args struct {
		ctx   context.Context
		state *rma.Tree
		tx    *types.TransactionOrder
		attr  *PCallAttributes
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "owner proof present",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramCall,
						UnitID:         counterProgramUnitID.Bytes(),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
					OwnerProof: testtransaction.RandomBytes(64),
				},
				attr: &PCallAttributes{
					FuncName:  "count",
					InputData: uint64ToLEBytes(1),
				},
			},
			wantErrStr: "owner proof present",
		},
		{
			name: "program not found",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramCall,
						UnitID:         testtransaction.RandomBytes(32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
					OwnerProof: nil,
				},
				attr: &PCallAttributes{
					FuncName:  "count",
					InputData: uint64ToLEBytes(1),
				},
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in program not found",
			args: args{
				ctx:   context.Background(),
				state: initStateWithBuiltInPrograms(t),
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramCall,
						UnitID:         make([]byte, 32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
					OwnerProof: nil,
				},
				attr: &PCallAttributes{
					FuncName:  "count",
					InputData: uint64ToLEBytes(1),
				},
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in ok",
			args: args{
				ctx:   context.Background(),
				state: initStateWithBuiltInPrograms(t),
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramCall,
						UnitID:         counterProgramUnitID.Bytes(),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
					OwnerProof: nil,
				},
				attr: &PCallAttributes{
					FuncName:  "count",
					InputData: uint64ToLEBytes(1),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handlePCallTx(tt.args.ctx, tt.args.state, systemID, crypto.SHA256)(tt.args.tx, tt.args.attr, 10)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func initStateWithBuiltInPrograms(t *testing.T) *rma.Tree {
	state := rma.NewWithSHA256()
	counterStateId := CreateStateDataID(counterProgramUnitID, util.Uint32ToBytes(counterState))
	cnt := make([]byte, 8)
	binary.LittleEndian.PutUint64(cnt, 1)
	// add both state and program
	require.NoError(t,
		state.AtomicUpdate(
			rma.AddItem(counterProgramUnitID,
				script.PredicateAlwaysFalse(),
				&Program{wasm: counterWasm, progParams: []byte{1, 0, 0, 0, 0, 0, 0, 0}},
				make([]byte, 32)),
			rma.AddItem(counterStateId,
				script.PredicateAlwaysFalse(),
				&Data{bytes: cnt},
				make([]byte, 32)),
		))
	state.Commit()
	return state
}
