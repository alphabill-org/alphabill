package state

import (
	"crypto"
	"testing"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestAdd(t *testing.T) {
	type args struct {
		id     types.UnitID
		bearer []byte
		data   types.UnitData
	}
	type testCase struct {
		name            string
		args            args
		initialState    *State
		executionErrStr string
		expectedUnit    *Unit
	}
	tests := []testCase{
		{
			name: "unit id is nil",
			args: args{
				id: nil,
			},
			initialState:    NewEmptyState(),
			executionErrStr: "id is nil",
		},
		{
			name: "unit ID exists",
			args: args{
				id:     []byte{1, 1, 1, 1},
				bearer: []byte{0x83, 0x00, 0x41, 0x01, 0xf6},
				data: &TestData{
					Value: 100,
				},
			},
			initialState:    newStateWithUnits(t),
			executionErrStr: "unable to add unit: key 01010101 exists",
		},
		{
			name: "ok",
			args: args{
				id:     []byte{1},
				bearer: []byte{0x83, 0x00, 0x41, 0x01, 0xf6},
				data:   &TestData{Value: 123, OwnerPredicate: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}},
			},
			initialState: NewEmptyState(),
			expectedUnit: func() *Unit {
				subTreeSumHash, err := abhash.HashValues(crypto.SHA256,
					[]byte{1},
					nil, // h_s is nil (we do not have a log entry)
					123,
					0,
					make([]byte, 32),
					0,
					make([]byte, 32),
				)
				require.NoError(t, err)
				return &Unit{
					logs:                nil,
					logsHash:            nil,
					data:                &TestData{Value: 123},
					subTreeSummaryValue: 123,
					subTreeSummaryHash:  subTreeSumHash,
				}
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add := AddUnit(tt.args.id, tt.args.data)
			err := add(tt.initialState.latestSavepoint(), crypto.SHA256)
			if tt.executionErrStr != "" {
				require.ErrorContains(t, err, tt.executionErrStr)
			}
			if tt.expectedUnit != nil {
				assertUnit(t, tt.initialState, tt.args.id, tt.expectedUnit, false)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		id types.UnitID
		f  UpdateFunction
	}
	type testCase struct {
		name            string
		args            args
		initialState    *State
		executionErrStr string
		expectedUnit    *Unit
	}
	tests := []testCase{
		{
			name: "not found",
			args: args{
				id: []byte{1},
				f: func(data types.UnitData) (types.UnitData, error) {
					return data, nil
				},
			},
			initialState:    NewEmptyState(),
			executionErrStr: "failed to get unit: item 01 does not exist: not found",
		},
		{
			name: "update function is nil",
			args: args{
				id: test.RandomBytes(32),
			},
			initialState:    NewEmptyState(),
			executionErrStr: "update function is nil",
		},
		{
			name: "ok",
			args: args{
				id: []byte{1, 1, 1, 1},
				f: func(data types.UnitData) (types.UnitData, error) {
					return &TestData{Value: 200, OwnerPredicate: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}, nil
				},
			},
			initialState: newStateWithUnits(t),
			expectedUnit: &Unit{
				logs:                nil,
				logsHash:            nil,
				data:                &TestData{Value: 200, OwnerPredicate: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}},
				subTreeSummaryValue: 10,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := UpdateUnitData(tt.args.id, tt.args.f)
			err := f(tt.initialState.latestSavepoint(), crypto.SHA256)
			if tt.executionErrStr != "" {
				require.ErrorContains(t, err, tt.executionErrStr)
			}
			if tt.expectedUnit != nil {
				assertUnit(t, tt.initialState, tt.args.id, tt.expectedUnit, false)
			}
		})
	}
}

func TestDelete(t *testing.T) {

	type testCase struct {
		name            string
		unitID          types.UnitID
		initialState    *State
		executionErrStr string
	}
	tests := []testCase{
		{
			name:            "unit ID is nil",
			unitID:          nil,
			initialState:    NewEmptyState(),
			executionErrStr: "id is nil",
		},
		{
			name:            "unit ID not found",
			unitID:          []byte{1},
			initialState:    NewEmptyState(),
			executionErrStr: "unable to delete unit",
		},
		{
			name:         "ok",
			unitID:       []byte{1, 1, 1, 1},
			initialState: newStateWithUnits(t),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DeleteUnit(tt.unitID)(tt.initialState.latestSavepoint(), crypto.SHA256)
			if tt.executionErrStr != "" {
				require.ErrorContains(t, err, tt.executionErrStr)
				return
			}
			require.NoError(t, err)
			u, err := tt.initialState.latestSavepoint().Get(tt.unitID)
			require.ErrorContains(t, err, "not found")
			require.Nil(t, u)
		})
	}
}

func Test_SetStateLock(t *testing.T) {
	t.Run("id is nil", func(t *testing.T) {
		err := SetStateLock(nil, []byte{})(NewEmptyState().latestSavepoint(), crypto.SHA256)
		require.Error(t, err)
		require.Equal(t, "id is nil", err.Error())
	})

	t.Run("unit not found", func(t *testing.T) {
		err := SetStateLock([]byte{1}, []byte{})(NewEmptyState().latestSavepoint(), crypto.SHA256)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to find unit")
	})

	t.Run("unit already has a state lock", func(t *testing.T) {
		s := newStateWithUnits(t)
		err := SetStateLock([]byte{1, 1, 1, 1}, []byte{1})(s.latestSavepoint(), crypto.SHA256)
		require.NoError(t, err)
		err = SetStateLock([]byte{1, 1, 1, 1}, []byte{1})(s.latestSavepoint(), crypto.SHA256)
		require.Error(t, err)
		require.Equal(t, "unit already has a state lock", err.Error())
	})

	t.Run("successful state lock set", func(t *testing.T) {
		s := newStateWithUnits(t).latestSavepoint()
		err := SetStateLock([]byte{1, 1, 1, 1}, []byte{1})(s, crypto.SHA256)
		require.NoError(t, err)
		u, _ := s.Get([]byte{1, 1, 1, 1})
		require.Equal(t, []byte{1}, u.GetV1().stateLockTx)
	})
}

func newStateWithUnits(t *testing.T) *State {
	s := NewEmptyState()
	require.NoError(t,
		s.Apply(
			AddUnit([]byte{1, 1, 1, 1}, &TestData{Value: 10, OwnerPredicate: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		),
	)
	return s
}

func assertUnit(t *testing.T, state *State, unitID types.UnitID, expectedUnit *Unit, committed bool) {
	t.Helper()
	unit, err := state.latestSavepoint().Get(unitID)
	require.NoError(t, err)
	require.NotNil(t, unit)
	assertUnitEqual(t, expectedUnit, unit.GetV1())

	committedUnit, err := state.committedTree.Get(unitID)
	if !committed {
		require.ErrorContains(t, err, "not found")
	} else {
		require.NoError(t, err)
		require.NotNil(t, committedUnit)
		assertUnitEqual(t, expectedUnit, unit.GetV1())
	}
}

func assertUnitEqual(t *testing.T, expectedUnit *Unit, unit *Unit) {
	require.Equal(t, expectedUnit.data, unit.data)
	require.Equal(t, expectedUnit.subTreeSummaryValue, unit.subTreeSummaryValue)
}
