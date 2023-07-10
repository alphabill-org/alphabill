package state

import (
	"crypto"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var unitIdentifiers = []types.UnitID{
	[]byte{0, 0, 0, 0},
	[]byte{0, 0, 0, 1},
	[]byte{0, 0, 0, 2},
	[]byte{0, 0, 0, 3},
	[]byte{0, 0, 0, 4},
	[]byte{0, 0, 0, 5},
	[]byte{0, 0, 0, 6},
	[]byte{0, 0, 0, 7},
	[]byte{0, 0, 0, 8},
	[]byte{0, 0, 0, 9},
	[]byte{0, 0, 1, 0},
}

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
	require.ErrorContains(t, s.Commit(), "call CalculateRoot method before committing a state")
}

func TestState_Apply_RevertsChangesAfterActionReturnsError(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewState(t)
	require.ErrorContains(t, s.Apply(
		AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData),
		UpdateUnitData([]byte{0, 0, 0, 2}, func(data UnitData) (UnitData, error) {
			return data, nil
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
	_, err := s.AddUnitLog(unitID, test.RandomBytes(32))
	require.NoError(t, err)

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
	_, err := s.AddUnitLog(unitID, test.RandomBytes(32))
	require.NoError(t, err)
	txrHash := test.RandomBytes(32)
	_, err = s.AddUnitLog(unitID, txrHash)
	require.NoError(t, err)

	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.Len(t, u.logs, 2)
	require.Equal(t, txrHash, u.logs[1].txRecordHash)
}

func TestState_CommitTreeWithLeftAndRightChildNodes(t *testing.T) {
	s := NewState(t)
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), &TestData{Value: 10})))
	_, err := s.AddUnitLog([]byte{0, 0, 0, 1}, test.RandomBytes(32))
	require.NoError(t, err)

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 2}, test.RandomBytes(20), &TestData{Value: 20})))
	_, err = s.AddUnitLog([]byte{0, 0, 0, 2}, test.RandomBytes(32))
	require.NoError(t, err)

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 3}, test.RandomBytes(20), &TestData{Value: 30})))
	_, err = s.AddUnitLog([]byte{0, 0, 0, 3}, test.RandomBytes(32))
	require.NoError(t, err)

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 4}, test.RandomBytes(20), &TestData{Value: 42})))
	_, err = s.AddUnitLog([]byte{0, 0, 0, 4}, test.RandomBytes(32))
	require.NoError(t, err)

	summary, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(102), summary)
	require.NotNil(t, rootHash)
}

func TestState_AddUnitLog_UnitDoesNotExist(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	s := NewState(t)
	_, err := s.AddUnitLog(unitID, test.RandomBytes(32))
	require.ErrorContains(t, err, "unable to add unit log for unit 00000001")
}

func TestState_PruneLog(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewState(t)

	require.NoError(t, s.Apply(AddUnit(unitID, test.RandomBytes(20), unitData)))
	_, err := s.AddUnitLog(unitID, test.RandomBytes(32))
	require.NoError(t, err)

	require.NoError(t, s.Apply(UpdateUnitData(unitID, func(data UnitData) (UnitData, error) {
		data.(*TestData).Value = 100
		return data, nil
	})))
	_, err = s.AddUnitLog(unitID, test.RandomBytes(32))
	require.NoError(t, err)
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

func TestCreateAndVerifyStateProofs_CreateUnits(t *testing.T) {
	s, stateRootHash, summaryValue := prepareState(t)
	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0, nil)
		require.NoError(t, err)

		proofOutputHash, sum := stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)
	}
}

func TestCreateAndVerifyStateProofs_UpdateUnits(t *testing.T) {
	s, stateRootHash, summaryValue := prepareState(t)
	stateRootHash, summaryValue = updateUnits(t, s, summaryValue, stateRootHash)
	require.Equal(t, uint64(5510), summaryValue)

	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0, nil)
		require.NoError(t, err)

		proofOutputHash, sum := stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)

		stateProof, err = s.CreateUnitStateProof(id, 1, nil)
		require.NoError(t, err)

		proofOutputHash, sum = stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)
	}
}

func TestCreateAndVerifyStateProofs_UpdateAndPruneUnits(t *testing.T) {
	s, stateRootHash, summaryValue := prepareState(t)
	stateRootHash, summaryValue = updateUnits(t, s, summaryValue, stateRootHash)
	require.Equal(t, uint64(5510), summaryValue)
	for _, id := range unitIdentifiers {
		require.NoError(t, s.PruneLog(id))
	}
	summaryValue, stateRootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	stateRootHash, summaryValue = updateUnits(t, s, summaryValue, stateRootHash)
	require.Equal(t, uint64(55100), summaryValue)

	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0, nil)
		require.NoError(t, err)

		proofOutputHash, sum := stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)

		stateProof, err = s.CreateUnitStateProof(id, 1, nil)
		require.NoError(t, err)

		proofOutputHash, sum = stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)
	}
}

func prepareState(t *testing.T) (*State, []byte, uint64) {
	s := newEmptyState(t)
	//			┌───┤ key=00000100
	//			│	└───┤ key=00000009
	//		┌───┤ key=00000008
	//		│	└───┤ key=00000007
	//	────┤ key=00000006
	//		│		┌───┤ key=00000005
	//		│	┌───┤ key=00000004
	//		└───┤ key=00000003
	//			│	┌───┤ key=00000002
	//			└───┤ key=00000001
	//				└───┤ key=00000000
	require.NoError(t, s.Apply(
		AddUnit([]byte{0, 0, 0, 1}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 10}),
		AddUnit([]byte{0, 0, 0, 6}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 60}),
		AddUnit([]byte{0, 0, 0, 2}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 20}),
		AddUnit([]byte{0, 0, 0, 3}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 30}),
		AddUnit([]byte{0, 0, 0, 7}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 70}),
		AddUnit([]byte{0, 0, 0, 4}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 40}),
		AddUnit([]byte{0, 0, 1, 0}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 100}),
		AddUnit([]byte{0, 0, 0, 8}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 80}),
		AddUnit([]byte{0, 0, 0, 5}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 50}),
		AddUnit([]byte{0, 0, 0, 9}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 90}),
		AddUnit([]byte{0, 0, 0, 0}, script.PredicateAlwaysTrue(), &pruneUnitData{i: 1}),
	))
	txrHash := test.RandomBytes(32)

	for _, id := range unitIdentifiers {
		addLog(t, s, id, txrHash)
	}

	sum, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(551), sum)
	require.NotNil(t, rootHash)
	require.NoError(t, s.Commit())
	return s, rootHash, sum
}

func updateUnits(t *testing.T, s *State, summaryValue uint64, stateRootHash []byte) ([]byte, uint64) {
	require.NoError(t, s.Apply(
		UpdateUnitData([]byte{0, 0, 0, 6}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 1}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 2}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 3}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 7}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 4}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 1, 0}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 8}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 5}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 9}, multiply(10)),
		UpdateUnitData([]byte{0, 0, 0, 0}, multiply(10)),
	))
	txrHash := test.RandomBytes(32)
	for _, id := range unitIdentifiers {
		addLog(t, s, id, txrHash)
	}
	summaryValue, stateRootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NotNil(t, stateRootHash)
	require.NoError(t, s.Commit())
	return stateRootHash, summaryValue
}

func multiply(t uint64) func(data UnitData) (UnitData, error) {
	return func(data UnitData) (UnitData, error) {
		d := data.(*pruneUnitData)
		d.i = d.i * t
		return d, nil
	}
}

func addLog(t *testing.T, s *State, id []byte, txrHash []byte) {
	_, err := s.AddUnitLog(id, txrHash)
	require.NoError(t, err)
}

func NewState(t *testing.T, opts ...Option) *State {
	s, err := New(opts...)
	require.NoError(t, err)
	return s
}
