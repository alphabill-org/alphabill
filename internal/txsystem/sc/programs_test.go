package sc

import (
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/stretchr/testify/require"
)

var unlockMoneyBillProgramUnitID = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 's', 'y', 's'}

func TestInitBuiltInPrograms(t *testing.T) {
	tests := []struct {
		name    string
		state   *state.State
		want    BuiltInPrograms
		wantErr string
	}{
		{
			name:  "init built-in programs",
			state: state.NewEmptyState(),
			want:  BuiltInPrograms{string(unlockMoneyBillProgramUnitID): unlockMoneyBill},
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

func initStateWithBuiltInPrograms(t *testing.T) *state.State {
	s := state.NewEmptyState()
	savepointID := s.Savepoint()
	require.NoError(t,
		s.Apply(state.AddUnit(
			unlockMoneyBillProgramUnitID,
			script.PredicateAlwaysFalse(),
			&Program{},
		)))
	s.ReleaseToSavepoint(savepointID)
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return s
}
