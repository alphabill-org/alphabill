package state

import (
	"crypto"
	"testing"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

var unitID = []byte{1, 1, 1, 1}

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
		expectedUnit    *UnitV1
	}
	ownerPredicate := []byte{0x83, 0x00, 0x41, 0x01, 0xf6}

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
				id:     unitID,
				bearer: ownerPredicate,
				data: &TestData{
					Value: 100,
				},
			},
			initialState:    newStateWithUnits(t),
			executionErrStr: "unable to add unit: node already exists: key 01010101 exists",
		},
		{
			name: "ok",
			args: args{
				id:     []byte{1},
				bearer: ownerPredicate,
				data:   &TestData{Value: 123, OwnerPredicate: ownerPredicate},
			},
			initialState: NewEmptyState(),
			expectedUnit: func() *UnitV1 {
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
				return &UnitV1{
					logs:                nil,
					logsHash:            nil,
					data:                &TestData{Value: 123, OwnerPredicate: ownerPredicate},
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
		expectedUnit    *UnitV1
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
				id: unitID,
				f: func(data types.UnitData) (types.UnitData, error) {
					return &TestData{Value: 200, OwnerPredicate: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}, nil
				},
			},
			initialState: newStateWithUnits(t),
			expectedUnit: &UnitV1{
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
			unitID:       unitID,
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
		err := SetStateLock(unitID, []byte{1})(s.latestSavepoint(), crypto.SHA256)
		require.NoError(t, err)
		err = SetStateLock(unitID, []byte{1})(s.latestSavepoint(), crypto.SHA256)
		require.Error(t, err)
		require.Equal(t, "unit already has a state lock", err.Error())
	})

	t.Run("successful state lock set", func(t *testing.T) {
		s := newStateWithUnits(t).latestSavepoint()
		err := SetStateLock(unitID, []byte{1})(s, crypto.SHA256)
		require.NoError(t, err)
		u, _ := s.Get(unitID)
		unit, err := ToUnitV1(u)
		require.NoError(t, err)
		require.Equal(t, []byte{1}, unit.stateLockTx)
	})
}

func Test_RemoveStateLock(t *testing.T) {
	stateLockTx := []byte{1}

	t.Run("id is nil", func(t *testing.T) {
		err := RemoveStateLock(nil)(NewEmptyState().latestSavepoint(), crypto.SHA256)
		require.Error(t, err)
		require.ErrorContains(t, err, "id is nil")
	})

	t.Run("unit not found", func(t *testing.T) {
		err := RemoveStateLock(unitID)(NewEmptyState().latestSavepoint(), crypto.SHA256)
		require.ErrorContains(t, err, "failed to find unit")
	})

	t.Run("unit does not have a state lock", func(t *testing.T) {
		s := newStateWithUnits(t)
		err := RemoveStateLock(unitID)(s.latestSavepoint(), crypto.SHA256)
		require.EqualError(t, err, "unit does not have a state lock")
	})

	t.Run("state lock removed successfully", func(t *testing.T) {
		s := newStateWithUnits(t).latestSavepoint()
		require.NoError(t, SetStateLock(unitID, stateLockTx)(s, crypto.SHA256))
		require.NoError(t, RemoveStateLock(unitID)(s, crypto.SHA256))
		u, err := s.Get(unitID)
		require.NoError(t, err)
		unit, err := ToUnitV1(u)
		require.NoError(t, err)
		require.Nil(t, unit.stateLockTx)
	})
}

func Test_AddOrPromoteUnit(t *testing.T) {
	unitData := &TestData{Value: 10, OwnerPredicate: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}

	t.Run("id is nil", func(t *testing.T) {
		err := AddOrPromoteUnit(nil, unitData)(NewEmptyState().latestSavepoint(), crypto.SHA256)
		require.Error(t, err)
		require.ErrorContains(t, err, "id is nil")
	})

	t.Run("add unit ok - dummy unit exists", func(t *testing.T) {
		s := newStateWithDummyUnits(t)
		add := AddOrPromoteUnit(unitID, unitData)
		err := add(s.latestSavepoint(), crypto.SHA256)
		require.NoError(t, err)
		u, err := s.GetUnit(unitID, false)
		require.NoError(t, err)
		unit, err := ToUnitV1(u)
		require.NoError(t, err)
		require.False(t, unit.IsDummy())
		require.Equal(t, unitData, unit.data)
	})

	t.Run("add unit ok - unit does not exist", func(t *testing.T) {
		s := NewEmptyState()
		err := AddOrPromoteUnit(unitID, unitData)(s.latestSavepoint(), crypto.SHA256)
		require.NoError(t, err)
		u, err := s.GetUnit(unitID, false)
		require.NoError(t, err)
		unit, err := ToUnitV1(u)
		require.NoError(t, err)
		require.Equal(t, unitData, unit.data)
	})

	t.Run("add unit nok - normal unit already exists", func(t *testing.T) {
		s := newStateWithUnits(t)
		err := AddOrPromoteUnit(unitID, unitData)(s.latestSavepoint(), crypto.SHA256)
		require.ErrorContains(t, err, "non-dummy unit already exists")
	})
}

func Test_MarkForDeletion(t *testing.T) {
	t.Run("id is nil", func(t *testing.T) {
		err := MarkForDeletion(nil, 0)(NewEmptyState().latestSavepoint(), crypto.SHA256)
		require.Error(t, err)
		require.ErrorContains(t, err, "id is nil")
	})

	t.Run("unit not found", func(t *testing.T) {
		err := MarkForDeletion(unitID, 0)(NewEmptyState().latestSavepoint(), crypto.SHA256)
		require.ErrorContains(t, err, "failed to find unit")
	})

	t.Run("ok", func(t *testing.T) {
		s := newStateWithUnits(t).latestSavepoint()
		require.NoError(t, MarkForDeletion(unitID, 10)(s, crypto.SHA256))
		u, err := s.Get(unitID)
		require.NoError(t, err)
		unit, err := ToUnitV1(u)
		require.NoError(t, err)
		require.EqualValues(t, 10, unit.deletionRound)
	})
}

func newStateWithUnits(t *testing.T) *State {
	s := NewEmptyState()
	require.NoError(t,
		s.Apply(
			AddUnit(unitID, &TestData{Value: 10, OwnerPredicate: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		),
	)
	return s
}

func newStateWithDummyUnits(t *testing.T) *State {
	s := NewEmptyState()
	require.NoError(t,
		s.Apply(
			AddDummyUnit(unitID),
		),
	)
	return s
}

func assertUnit(t *testing.T, state *State, unitID types.UnitID, expectedUnit *UnitV1, committed bool) {
	t.Helper()
	unit, err := state.latestSavepoint().Get(unitID)
	require.NoError(t, err)
	require.NotNil(t, unit)
	u, err := ToUnitV1(unit)
	require.NoError(t, err)
	assertUnitEqual(t, expectedUnit, u)

	committedUnit, err := state.committedTree.Get(unitID)
	if !committed {
		require.ErrorContains(t, err, "not found")
	} else {
		require.NoError(t, err)
		require.NotNil(t, committedUnit)
		u, err = ToUnitV1(unit)
		require.NoError(t, err)
		assertUnitEqual(t, expectedUnit, u)
	}
}

func assertUnitEqual(t *testing.T, expectedUnit *UnitV1, unit *UnitV1) {
	require.Equal(t, expectedUnit.data, unit.data)
	require.Equal(t, expectedUnit.subTreeSummaryValue, unit.subTreeSummaryValue)
}
