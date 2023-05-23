package program

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var counterProgramUnitID = uint256.NewInt(0).SetBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 's', 'y', 's'})
var counterState uint32 = 0xaabbccdd

func TestInitBuiltInPrograms(t *testing.T) {
	tests := []struct {
		name    string
		state   *rma.Tree
		want    []byte
		wantErr string
	}{
		{
			name:  "init built-in programs",
			state: rma.NewWithSHA256(),
			want:  counterWasm,
		},
		{
			name:    "state is nil",
			wantErr: "state is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := initBuiltInPrograms(tt.state)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			unit, err := tt.state.GetUnit(counterProgramUnitID)
			require.NoError(t, err)
			prog, ok := unit.Data.(*Program)
			require.True(t, ok)
			require.NotEmpty(t, prog)
			require.Equal(t, len(tt.want), len(prog.Wasm()))
			require.True(t, bytes.Equal(prog.wasm, counterWasm))
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
				&Program{wasm: counterWasm, initData: []byte{}},
				make([]byte, 32)),
			rma.AddItem(counterStateId,
				script.PredicateAlwaysFalse(),
				&StateFile{bytes: cnt},
				make([]byte, 32)),
		))
	state.Commit()
	return state
}
