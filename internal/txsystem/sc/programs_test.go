package sc

import (
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var unlockMoneyBillProgramUnitID = uint256.NewInt(0).SetBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 's', 'y', 's'})

func TestInitBuiltInPrograms(t *testing.T) {
	tests := []struct {
		name    string
		state   *rma.Tree
		want    BuiltInPrograms
		wantErr string
	}{
		{
			name:  "init built-in programs",
			state: rma.NewWithSHA256(),
			want:  BuiltInPrograms{string(util.Uint256ToBytes(unlockMoneyBillProgramUnitID)): unlockMoneyBill},
		},
		{
			name:    "state is nil",
			wantErr: "state is nil",
		},
		{
			name:    "built-in programs exists",
			state:   initStateWithBuiltInPrograms(t),
			wantErr: "failed to add 'unlock money bill' program to the state",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := initBuiltInPrograms(tt.state)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.Equal(t, len(tt.want), len(got))
			for s, program := range tt.want {
				p, f := got[s]
				require.True(t, f)
				require.Equal(t, reflect.ValueOf(program).Pointer(), reflect.ValueOf(p).Pointer())
			}
		})
	}
}

func TestUnlockMoneyBill(t *testing.T) {
	require.ErrorContains(t, unlockMoneyBill(nil), "not implemented")
}

func initStateWithBuiltInPrograms(t *testing.T) *rma.Tree {
	state := rma.NewWithSHA256()
	require.NoError(t,
		state.AtomicUpdate(rma.AddItem(
			unlockMoneyBillProgramUnitID,
			script.PredicateAlwaysFalse(),
			&Program{},
			make([]byte, 32),
		)))
	state.Commit()
	return state
}
