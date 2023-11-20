package state

import (
	"crypto"
	"fmt"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
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
	_     struct{} `cbor:",toarray"`
	Value uint64
}

func (t *TestData) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(t)
	if err != nil {
		return fmt.Errorf("test data serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
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
	spID := s.Savepoint()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	s.ReleaseToSavepoint(spID)

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
	spID := s.Savepoint()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	s.RollbackToSavepoint(spID)

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

func TestState_NestedSavepointsCommitsAndReverts(t *testing.T) {
	s := NewState(t)
	id := s.Savepoint()
	require.NoError(t,
		s.Apply(AddUnit([]byte{0, 0, 0, 0}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 2}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 3}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 4}, test.RandomBytes(20), &TestData{Value: 1})),
	)
	s.ReleaseToSavepoint(id)

	require.False(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)

	summary, rootHash, err := s.CalculateRoot()

	require.NoError(t, err)
	require.NoError(t, s.Commit())
	//	 		┌───┤ key=00000004, depth=1, summaryCalculated=true, nodeSummary=1, subtreeSummary=1, clean=true
	//		┌───┤ key=00000003, depth=2, summaryCalculated=true, nodeSummary=1, subtreeSummary=3, clean=true
	//		│	└───┤ key=00000002, depth=1, summaryCalculated=true, nodeSummary=1, subtreeSummary=1, clean=true
	//	────┤ key=00000001, depth=3, summaryCalculated=true, nodeSummary=1, subtreeSummary=5, clean=true
	//		└───┤ key=00000000, depth=1, summaryCalculated=true, nodeSummary=1, subtreeSummary=1, clean=true
	require.Equal(t, uint64(5), summary)
	require.True(t, s.IsCommitted())
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)

	id2 := s.Savepoint()
	id3 := s.Savepoint()
	require.NoError(t,
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 3}, func(data UnitData) (UnitData, error) {
			data.(*TestData).Value = 2
			return data, nil
		})),
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 0}, func(data UnitData) (UnitData, error) {
			data.(*TestData).Value = 2
			return data, nil
		})),
	)
	s.ReleaseToSavepoint(id3)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	id4 := s.Savepoint()
	require.NoError(t,
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 3}, func(data UnitData) (UnitData, error) {
			data.(*TestData).Value = 4
			return data, nil
		})),
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 0}, func(data UnitData) (UnitData, error) {
			data.(*TestData).Value = 4
			return data, nil
		})),
	)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)

	summary, _, err = s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(11), summary)
	require.False(t, s.IsCommitted())
	s.RollbackToSavepoint(id4)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	summary, _, err = s.CalculateRoot()
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	require.NoError(t, err)
	require.Equal(t, uint64(7), summary)
	require.False(t, s.IsCommitted())
	s.RollbackToSavepoint(id2)

	summary, rootHash2, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	require.NoError(t, err)
	require.Equal(t, uint64(5), summary)
	require.True(t, s.IsCommitted())
	require.Equal(t, rootHash, rootHash2)
}

func TestState_NestedSavepointsWithRemoveOperation(t *testing.T) {
	s := NewState(t)
	id := s.Savepoint()
	require.NoError(t,
		s.Apply(AddUnit([]byte{0, 0, 0, 0}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 2}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 3}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 4}, test.RandomBytes(20), &TestData{Value: 1})),
	)
	s.ReleaseToSavepoint(id)
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	id = s.Savepoint()
	require.NoError(t,
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 3}, func(data UnitData) (UnitData, error) {
			data.(*TestData).Value = 2
			return data, nil
		})),
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 4}, func(data UnitData) (UnitData, error) {
			data.(*TestData).Value = 2
			return data, nil
		})),
	)
	id2 := s.Savepoint()
	require.NoError(t,
		s.Apply(DeleteUnit([]byte{0, 0, 0, 1})),
	)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)

	s.RollbackToSavepoint(id2)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	id2 = s.Savepoint()
	require.NoError(t,
		s.Apply(DeleteUnit([]byte{0, 0, 0, 2})),
	)
	s.ReleaseToSavepoint(id2)
	s.ReleaseToSavepoint(id)
	summary, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	require.Equal(t, uint64(6), summary)
}

func TestState_RevertAVLTreeRotations(t *testing.T) {
	s := NewState(t)
	// initial state:
	// 		┌───┤ key=0000001E, depth=1, nodeSummary=30, subtreeSummary=30,
	//	────┤ key=00000014, depth=3, nodeSummary=20, subtreeSummary=76,
	//		│	┌───┤ key=0000000F, depth=1, nodeSummary=15, subtreeSummary=15,
	//		└───┤ key=0000000A, depth=2, nodeSummary=10, subtreeSummary=26,
	//			└───┤ key=00000001, depth=1, nodeSummary=1, subtreeSummary=1,
	require.NoError(t,
		s.Apply(AddUnit([]byte{0, 0, 0, 20}, test.RandomBytes(20), &TestData{Value: 20})),
		s.Apply(AddUnit([]byte{0, 0, 0, 10}, test.RandomBytes(20), &TestData{Value: 10})),
		s.Apply(AddUnit([]byte{0, 0, 0, 30}, test.RandomBytes(20), &TestData{Value: 30})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 15}, test.RandomBytes(20), &TestData{Value: 15})),
	)

	// commit initial state
	_, root, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	require.NoError(t,
		// change the unit that will be rotated
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 15}, func(data UnitData) (UnitData, error) {
			data.(*TestData).Value = 30
			return data, nil
		})),
		// rotate left right
		s.Apply(AddUnit([]byte{0, 0, 0, 12}, test.RandomBytes(20), &TestData{Value: 12})),
	)

	// calculate root after applying changes
	_, _, err = s.CalculateRoot()
	require.NoError(t, err)

	s.Revert()
	_, root2, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, root, root2)
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
	s, _, _ := prepareState(t)
	stateRootHash, summaryValue := updateUnits(t, s)
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
	s, _, _ := prepareState(t)
	_, summaryValue := updateUnits(t, s)
	require.Equal(t, uint64(5510), summaryValue)
	for _, id := range unitIdentifiers {
		require.NoError(t, s.PruneLog(id))
	}
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	stateRootHash, summaryValue := updateUnits(t, s)
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
		AddUnit([]byte{0, 0, 0, 1}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 10}),
		AddUnit([]byte{0, 0, 0, 6}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 60}),
		AddUnit([]byte{0, 0, 0, 2}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 20}),
		AddUnit([]byte{0, 0, 0, 3}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 30}),
		AddUnit([]byte{0, 0, 0, 7}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 70}),
		AddUnit([]byte{0, 0, 0, 4}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 40}),
		AddUnit([]byte{0, 0, 1, 0}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 100}),
		AddUnit([]byte{0, 0, 0, 8}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 80}),
		AddUnit([]byte{0, 0, 0, 5}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 50}),
		AddUnit([]byte{0, 0, 0, 9}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 90}),
		AddUnit([]byte{0, 0, 0, 0}, templates.AlwaysTrueBytes(), &pruneUnitData{i: 1}),
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

func updateUnits(t *testing.T, s *State) ([]byte, uint64) {
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

func getUnit(t *testing.T, s *State, id []byte) *Unit {
	unit, err := s.latestSavepoint().Get(id)
	require.NoError(t, err)
	return unit
}

func NewState(t *testing.T, opts ...Option) *State {
	s, err := New(opts...)
	require.NoError(t, err)
	return s
}
