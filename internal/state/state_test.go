package state

import (
	"crypto"
	"hash"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

type TestData struct {
	Value uint64
}

func (t *TestData) Write(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(t.Value))
}

func (t *TestData) SummaryValueInput() uint64 {
	return t.Value
}

func (t *TestData) Copy() UnitData {
	return &TestData{Value: t.Value}
}

func TestNewEmptyState(t *testing.T) {
	s := NewEmptyState()
	require.Nil(t, s.committedTree.Root())
	require.Nil(t, s.latestSavepoint().Root())
	require.Len(t, s.savepoints, 1)
	require.Equal(t, crypto.SHA256, s.hashAlgorithm)
	require.True(t, s.IsCommitted())
}

func TestNewStateWithSHA512(t *testing.T) {
	s, err := New(WithHashAlgorithm(crypto.SHA512))
	require.NoError(t, err)
	require.Equal(t, crypto.SHA512, s.hashAlgorithm)
}

func TestNewStateWithInitActions(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t, WithInitActions(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.NotNil(t, committedRoot)
	require.NotNil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
	require.Equal(t, unitData, uncommittedRoot.Value().data)
	require.Equal(t, unitData, committedRoot.Value().data)
	require.Equal(t, committedRoot, uncommittedRoot)
	require.True(t, s.IsCommitted())
}

func TestState_Savepoint_OK(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t)
	s.Savepoint()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	s.ReleaseSavepoint()

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.Nil(t, committedRoot)
	require.NotNil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
	require.Equal(t, unitData, uncommittedRoot.Value().data)
}

func TestState_RollbackSavepoint(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t)
	s.Savepoint()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	s.RollbackSavepoint()

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.Nil(t, committedRoot)
	require.Nil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
}

func TestState_Commit_OK(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t)
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.NotNil(t, committedRoot)
	require.NotNil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
	require.Equal(t, unitData, uncommittedRoot.Value().data)
	require.Equal(t, unitData, committedRoot.Value().data)
	require.Equal(t, committedRoot, uncommittedRoot)
}

func TestState_Commit_RootNotCalculated(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t)
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	require.ErrorContains(t, s.Commit(), "call CalculateRoot method befaore commiting a state")
}

func TestState_Apply_RevertsChangesAfterActionReturnsError(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t)
	require.ErrorContains(t, s.Apply(
		AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData),
		UpdateUnitData([]byte{0, 0, 0, 2}, func(data UnitData) (newData UnitData) {
			return data
		})), "failed to get unit: item 00000002")

	u, err := s.GetUnit([]byte{0, 0, 0, 1}, false)
	require.ErrorContains(t, err, "item 00000001 does not exist")
	require.Nil(t, u)
}

func TestState_Revert(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t)
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	s.Revert()

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.Nil(t, committedRoot)
	require.Nil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
}

func TestState_GetUnit(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewState(t)

	require.NoError(t, s.Apply(AddUnit(unitID, test.RandomBytes(20), unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))

	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	u, err = s.GetUnit(unitID, true)
	require.ErrorContains(t, err, "item 00000001 does not exist")
	require.Nil(t, u)

	_, _, err = s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	u, err = s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	u2, err := s.GetUnit(unitID, true)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, u, u2)
}

func TestState_AddUnitLog_OK(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewState(t)

	require.NoError(t, s.Apply(AddUnit(unitID, test.RandomBytes(20), unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))
	txrHash := test.RandomBytes(32)
	require.NoError(t, s.AddUnitLog(unitID, txrHash))

	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.Len(t, u.logs, 2)
	require.Equal(t, txrHash, u.logs[1].txRecordHash)
}

func TestState_CommitTreeWithLeftAndRightChildNodes(t *testing.T) {
	s := NewState(t)
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), &TestData{Value: 10})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 1}, test.RandomBytes(32)))

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 2}, test.RandomBytes(20), &TestData{Value: 20})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 2}, test.RandomBytes(32)))

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 3}, test.RandomBytes(20), &TestData{Value: 30})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 3}, test.RandomBytes(32)))

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 4}, test.RandomBytes(20), &TestData{Value: 42})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 4}, test.RandomBytes(32)))

	summary, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(102), summary)
	require.NotNil(t, rootHash)
}

func TestState_AddUnitLog_UnitDoesNotExist(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	s := NewState(t)
	require.ErrorContains(t, s.AddUnitLog(unitID, test.RandomBytes(32)), "unable to add unit log for unit 00000001")
}

func TestState_PruneLog(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewState(t)

	require.NoError(t, s.Apply(AddUnit(unitID, test.RandomBytes(20), unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))

	require.NoError(t, s.Apply(UpdateUnitData(unitID, func(data UnitData) UnitData {
		data.(*TestData).Value = 100
		return data
	})))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))
	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.Len(t, u.logs, 2)
	require.NoError(t, s.PruneLog(unitID))
	u, err = s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.Len(t, u.logs, 1)
	require.Nil(t, u.logs[0].txRecordHash)
}

func TestState_PruneLog_UnitNotFound(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	s := NewState(t)
	require.ErrorContains(t, s.PruneLog(unitID), "item 00000001 does not exist")
}

func NewState(t *testing.T, opts ...Option) *State {
	s, err := New(opts...)
	require.NoError(t, err)
	return s
}
