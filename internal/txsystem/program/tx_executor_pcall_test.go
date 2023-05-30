package program

import (
	"context"
	"crypto"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var systemID = []byte{0, 0, 0, 3}

var counterProgramUnitID = uint256.NewInt(0).SetBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'c', 'o', 'u', 'n', 't', 'e', 'r'})
var counterState uint32 = 0xaabbccdd

//go:embed test_program/counter.wasm
var counterWasm []byte

func Test_handlePCallTx(t *testing.T) {
	type args struct {
		ctx              context.Context
		state            *rma.Tree
		transactionOrder *PCallTransactionOrder
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
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(testtransaction.RandomBytes(64))),
			},
			wantErrStr: "owner proof present",
		},
		{
			name: "program not found",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithUnitId(make([]byte, 32)),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in program not found",
			args: args{
				ctx:   context.Background(),
				state: initStateWithBuiltInPrograms(t),
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithUnitId(uint256.NewInt(0).PaddedBytes(32)),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in ok",
			args: args{
				ctx:   context.Background(),
				state: initStateWithBuiltInPrograms(t),
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithUnitId(counterProgramUnitID.Bytes()),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handlePCallTx(tt.args.ctx, tt.args.state, systemID, crypto.SHA256)(tt.args.transactionOrder, 10)
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
	counterStateId := txutil.SameShardID(counterProgramUnitID, sha256.New().Sum(util.Uint32ToBytes(counterState)))
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
				&StateFile{bytes: cnt},
				make([]byte, 32)),
		))
	state.Commit()
	return state
}

func newPCallTxOrder(t *testing.T, fName string, opts ...testtransaction.Option) *PCallTransactionOrder {
	order := testtransaction.NewTransaction(t, opts...)
	return &PCallTransactionOrder{
		txOrder: order,
		attributes: &PCallAttributes{
			Function: fName,
			Input:    uint64ToLEBytes(1),
		},
	}
}
