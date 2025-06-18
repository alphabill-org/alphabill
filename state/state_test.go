package state

import (
	"bytes"
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	test "github.com/alphabill-org/alphabill/internal/testutils"
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
	_              struct{} `cbor:",toarray"`
	Value          uint64
	OwnerPredicate []byte
}

func (t *TestData) Write(hasher abhash.Hasher) {
	hasher.Write(t)
}

func (t *TestData) SummaryValueInput() uint64 {
	return t.Value
}

func (t *TestData) Copy() types.UnitData {
	return &TestData{Value: t.Value, OwnerPredicate: t.OwnerPredicate}
}

func (t *TestData) Owner() []byte {
	return t.OwnerPredicate
}

func (t *TestData) GetVersion() types.ABVersion {
	return 0
}

func TestNewEmptyState(t *testing.T) {
	s := NewEmptyState()
	require.Nil(t, s.committedTree.Root())
	require.Nil(t, s.latestSavepoint().Root())
	require.Len(t, s.savepoints, 1)
	require.Equal(t, crypto.SHA256, s.hashAlgorithm)
	committed, err := s.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
}

func TestNewStateWithSHA512(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA512))
	require.Equal(t, crypto.SHA512, s.hashAlgorithm)
}

func TestState_Savepoint_OK(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	spID, err := s.Savepoint()
	require.NoError(t, err)
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, unitData)))
	s.ReleaseToSavepoint(spID)

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.Nil(t, committedRoot)
	require.NotNil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
	require.Equal(t, unitData, uncommittedRoot.Value().Data())
}

func TestState_RollbackSavepoint(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	spID, err := s.Savepoint()
	require.NoError(t, err)
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, unitData)))
	s.RollbackToSavepoint(spID)

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.Nil(t, committedRoot)
	require.Nil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
}

func TestState_Commit_OK(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, unitData)))

	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	committed, err := s.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
	require.Nil(t, s.CommittedUC())

	require.NoError(t, s.Commit(createUC(t, s, summaryValue, summaryHash)))
	committed, err = s.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
	require.NotNil(t, s.CommittedUC())
	require.EqualValues(t, 1, s.CommittedUC().GetRoundNumber())

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.NotNil(t, committedRoot)
	require.NotNil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
	require.Equal(t, unitData, uncommittedRoot.Value().Data())
	require.Equal(t, unitData, committedRoot.Value().Data())
	require.Equal(t, committedRoot, uncommittedRoot)
}

func TestState_Commit_RootNotCalculated(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, unitData)))

	require.ErrorContains(t, s.Commit(createUC(t, s, 0, nil)), "call CalculateRoot method before committing a state")
	committed, err := s.IsCommitted()
	require.NoError(t, err)
	require.False(t, committed)
	require.Nil(t, s.CommittedUC())
}

func TestState_Commit_InvalidUC(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, unitData)))
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.ErrorContains(t, s.Commit(createUC(t, s, summaryValue, nil)), "state summary hash is not equal to the summary hash in UC")
	committed, err := s.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
	require.ErrorContains(t, s.Commit(createUC(t, s, 0, summaryHash)), "state summary value is not equal to the summary value in UC")
	committed, err = s.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
	require.Nil(t, s.CommittedUC())
}

func TestState_Apply_RevertsChangesAfterActionReturnsError(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	require.ErrorContains(t, s.Apply(
		AddUnit([]byte{0, 0, 0, 1}, unitData),
		UpdateUnitData([]byte{0, 0, 0, 2}, func(data types.UnitData) (types.UnitData, error) {
			return data, nil
		})), "failed to get unit: item 00000002")

	u, err := s.GetUnit([]byte{0, 0, 0, 1}, false)
	require.ErrorContains(t, err, "item 00000001 does not exist")
	require.Nil(t, u)
}

func TestState_Revert(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, unitData)))
	s.Revert()

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.Nil(t, committedRoot)
	require.Nil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
}

func TestState_NestedSavepointsCommitsAndReverts(t *testing.T) {
	s := NewEmptyState()
	id, err := s.Savepoint()
	require.NoError(t, err)
	require.NoError(t,
		s.Apply(AddUnit([]byte{0, 0, 0, 0}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 2}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 3}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 4}, &TestData{Value: 1})),
	)
	s.ReleaseToSavepoint(id)

	require.False(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.False(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)

	summary, rootHash, err := s.CalculateRoot()

	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, summary, rootHash)))
	//	 		┌───┤ key=00000004, depth=1, summaryCalculated=true, nodeSummary=1, subtreeSummary=1, clean=true
	//		┌───┤ key=00000003, depth=2, summaryCalculated=true, nodeSummary=1, subtreeSummary=3, clean=true
	//		│	└───┤ key=00000002, depth=1, summaryCalculated=true, nodeSummary=1, subtreeSummary=1, clean=true
	//	────┤ key=00000001, depth=3, summaryCalculated=true, nodeSummary=1, subtreeSummary=5, clean=true
	//		└───┤ key=00000000, depth=1, summaryCalculated=true, nodeSummary=1, subtreeSummary=1, clean=true
	require.Equal(t, uint64(5), summary)
	committed, err := s.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)

	id2, err := s.Savepoint()
	require.NoError(t, err)
	id3, err := s.Savepoint()
	require.NoError(t, err)
	require.NoError(t,
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 3}, func(data types.UnitData) (types.UnitData, error) {
			data.(*TestData).Value = 2
			return data, nil
		})),
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 0}, func(data types.UnitData) (types.UnitData, error) {
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
	id4, err := s.Savepoint()
	require.NoError(t, err)
	require.NoError(t,
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 3}, func(data types.UnitData) (types.UnitData, error) {
			data.(*TestData).Value = 4
			return data, nil
		})),
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 0}, func(data types.UnitData) (types.UnitData, error) {
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
	committed, err = s.IsCommitted()
	require.NoError(t, err)
	require.False(t, committed)
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
	committed, err = s.IsCommitted()
	require.NoError(t, err)
	require.False(t, committed)
	s.RollbackToSavepoint(id2)

	summary, rootHash2, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, summary, rootHash2)))
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	require.NoError(t, err)
	require.Equal(t, uint64(5), summary)
	committed, err = s.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
	require.Equal(t, rootHash, rootHash2)
}

func TestState_NestedSavepointsWithRemoveOperation(t *testing.T) {
	s := NewEmptyState()
	id, err := s.Savepoint()
	require.NoError(t, err)
	require.NoError(t,
		s.Apply(AddUnit([]byte{0, 0, 0, 0}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 2}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 3}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 4}, &TestData{Value: 1})),
	)
	s.ReleaseToSavepoint(id)
	value, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, value, hash)))
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 2}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	id, err = s.Savepoint()
	require.NoError(t, err)
	require.NoError(t,
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 3}, func(data types.UnitData) (types.UnitData, error) {
			data.(*TestData).Value = 2
			return data, nil
		})),
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 4}, func(data types.UnitData) (types.UnitData, error) {
			data.(*TestData).Value = 2
			return data, nil
		})),
	)
	id2, err := s.Savepoint()
	require.NoError(t, err)
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
	id2, err = s.Savepoint()
	require.NoError(t, err)
	require.NoError(t,
		s.Apply(DeleteUnit([]byte{0, 0, 0, 2})),
	)
	s.ReleaseToSavepoint(id2)
	s.ReleaseToSavepoint(id)
	summary, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, summary, hash)))
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 0}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 1}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 3}).summaryCalculated)
	require.True(t, getUnit(t, s, []byte{0, 0, 0, 4}).summaryCalculated)
	require.Equal(t, uint64(6), summary)
}

func TestState_RevertAVLTreeRotations(t *testing.T) {
	s := NewEmptyState()
	// initial state:
	// 		┌───┤ key=0000001E, depth=1, nodeSummary=30, subtreeSummary=30,
	//	────┤ key=00000014, depth=3, nodeSummary=20, subtreeSummary=76,
	//		│	┌───┤ key=0000000F, depth=1, nodeSummary=15, subtreeSummary=15,
	//		└───┤ key=0000000A, depth=2, nodeSummary=10, subtreeSummary=26,
	//			└───┤ key=00000001, depth=1, nodeSummary=1, subtreeSummary=1,
	require.NoError(t,
		s.Apply(AddUnit([]byte{0, 0, 0, 20}, &TestData{Value: 20})),
		s.Apply(AddUnit([]byte{0, 0, 0, 10}, &TestData{Value: 10})),
		s.Apply(AddUnit([]byte{0, 0, 0, 30}, &TestData{Value: 30})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 15}, &TestData{Value: 15})),
	)

	// commit initial state
	value, root, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, value, root)))

	require.NoError(t,
		// change the unit that will be rotated
		s.Apply(UpdateUnitData([]byte{0, 0, 0, 15}, func(data types.UnitData) (types.UnitData, error) {
			data.(*TestData).Value = 30
			return data, nil
		})),
		// rotate left right
		s.Apply(AddUnit([]byte{0, 0, 0, 12}, &TestData{Value: 12})),
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
	s := NewEmptyState()

	require.NoError(t, s.Apply(AddUnit(unitID, unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))

	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	u, err = s.GetUnit(unitID, true)
	require.ErrorContains(t, err, "item 00000001 does not exist")
	require.Nil(t, u)

	value, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, value, hash)))

	u, err = s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	unit1, err := ToUnitV1(u)
	require.NoError(t, err)

	u2, err := s.GetUnit(unitID, true)
	require.NoError(t, err)
	require.NotNil(t, u2)
	unit2, err := ToUnitV1(u2)
	require.NoError(t, err)
	// logRoot, subTreeSummaryHash and summaryCalculated do not get cloned - rest must match
	require.Equal(t, unit1.logs, unit2.logs)
	require.Equal(t, u.Data(), u2.Data())
	require.Equal(t, unit1.subTreeSummaryValue, unit2.subTreeSummaryValue)
}

func TestState_AddUnitLog_OK(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewEmptyState()

	require.NoError(t, s.Apply(AddUnit(unitID, unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))
	txrHash := test.RandomBytes(32)
	require.NoError(t, s.AddUnitLog(unitID, txrHash))

	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	unit, err := ToUnitV1(u)
	require.NoError(t, err)
	require.Len(t, unit.logs, 2)
	require.Equal(t, txrHash, unit.logs[1].TxRecordHash)
}

func TestState_CommitTreeWithLeftAndRightChildNodes(t *testing.T) {
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, &TestData{Value: 10})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 1}, test.RandomBytes(32)))

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 2}, &TestData{Value: 20})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 2}, test.RandomBytes(32)))

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 3}, &TestData{Value: 30})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 3}, test.RandomBytes(32)))

	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 4}, &TestData{Value: 42})))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 4}, test.RandomBytes(32)))

	summary, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(102), summary)
	require.NotNil(t, rootHash)
}

func TestState_AddUnitLog_UnitDoesNotExist(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	s := NewEmptyState()
	require.ErrorContains(t, s.AddUnitLog(unitID, test.RandomBytes(32)), "unable to find unit: item 00000001 does not exist")
}

func TestState_PruneState(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewEmptyState()

	require.NoError(t, s.Apply(AddUnit(unitID, unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))

	require.NoError(t, s.Apply(UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
		data.(*TestData).Value = 100
		return data, nil
	})))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))
	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	unit, err := ToUnitV1(u)
	require.NoError(t, err)
	require.Len(t, unit.logs, 2)
	require.NoError(t, s.Prune())
	u, err = s.GetUnit(unitID, false)
	require.NoError(t, err)
	unit, err = ToUnitV1(u)
	require.NoError(t, err)
	require.Len(t, unit.logs, 1)
	require.Nil(t, unit.logs[0].TxRecordHash)
}

func TestCreateAndVerifyStateProofs_CreateUnits(t *testing.T) {
	s, stateRootHash, summaryValue := prepareState(t)
	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0)
		require.NoError(t, err)

		proofOutputHash, sum, err := stateProof.CalculateStateTreeOutput(s.hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)
	}
}

func TestCreateAndVerifyStateProofs_UpdateUnits(t *testing.T) {
	s, _, _ := prepareState(t)
	stateRootHash, summaryValue := updateUnits(t, s)
	require.Equal(t, uint64(5510), summaryValue)

	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0)
		require.NoError(t, err)

		proofOutputHash, sum, err := stateProof.CalculateStateTreeOutput(s.hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)

		stateProof, err = s.CreateUnitStateProof(id, 1)
		require.NoError(t, err)

		proofOutputHash, sum, err = stateProof.CalculateStateTreeOutput(s.hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)
	}
}

func TestCreateAndVerifyStateProofs_UpdateAndPruneUnits(t *testing.T) {
	s, _, _ := prepareState(t)
	_, summaryValue := updateUnits(t, s)
	require.Equal(t, uint64(5510), summaryValue)
	require.NoError(t, s.Prune())
	value, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, value, hash)))
	stateRootHash, summaryValue := updateUnits(t, s)
	require.Equal(t, uint64(55100), summaryValue)

	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0)
		require.NoError(t, err)

		proofOutputHash, sum, err := stateProof.CalculateStateTreeOutput(s.hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)

		stateProof, err = s.CreateUnitStateProof(id, 1)
		require.NoError(t, err)

		proofOutputHash, sum, err = stateProof.CalculateStateTreeOutput(s.hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)
	}
}

type alwaysValid struct{}

func (a *alwaysValid) Validate(*types.UnicityCertificate, []byte) error {
	return nil
}

func TestCreateAndVerifyStateProofs_CreateUnitProof(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s, _, _ := prepareState(t)
		proof, err := s.CreateUnitStateProof([]byte{0, 0, 0, 5}, 0)
		require.NoError(t, err)
		u, err := s.GetUnit([]byte{0, 0, 0, 5}, true)
		require.NoError(t, err)
		unit, err := ToUnitV1(u)
		require.NoError(t, err)
		data, err := types.NewUnitState(unit.data, 0, nil)
		require.NoError(t, err)
		require.NoError(t, proof.Verify(crypto.SHA256, data, &alwaysValid{}, nil))
	})
	t.Run("unit not found", func(t *testing.T) {
		s, _, _ := prepareState(t)
		proof, err := s.CreateUnitStateProof([]byte{1, 0, 0, 5}, 0)
		require.ErrorContains(t, err, "unable to get unit 01000005: item 01000005 does not exist: not found")
		require.Nil(t, proof)
	})
}

func TestCreateAndVerifyStateProofs_CreateUnitProof_InvalidSummaryValue(t *testing.T) {
}

func TestSerialize_OK(t *testing.T) {
	s, _, _ := prepareState(t)
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 0}, []byte{1}))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 0}, []byte{2}))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 0}, []byte{3}))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 0}, []byte{4}))
	require.NoError(t, s.AddUnitLog([]byte{0, 0, 0, 0}, []byte{5}))

	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	uc := createUC(t, s, summaryValue, summaryHash)
	require.NoError(t, s.Commit(uc))

	buf := &bytes.Buffer{}
	executedTransactions := map[string]uint64{"tx1": 1, "tx2": 2, "tx3": 3}
	// Writes the pruned state
	require.NoError(t, s.Serialize(buf, true, executedTransactions))

	recoveredState, header, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)

	recoveredSummaryValue, recoveredSummaryHash, err := recoveredState.CalculateRoot()
	require.NoError(t, err)
	committed, err := recoveredState.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
	require.Equal(t, summaryValue, recoveredSummaryValue)
	require.Equal(t, summaryHash, recoveredSummaryHash)
	require.Equal(t, uc, recoveredState.CommittedUC())
	require.Equal(t, executedTransactions, header.ExecutedTransactions)
}

func TestSerialize_InvalidHeader(t *testing.T) {
	s, _, _ := prepareState(t)

	buf := &bytes.Buffer{}
	// Writes the pruned state
	require.NoError(t, s.Serialize(buf, true, nil))

	_, err := buf.ReadByte()
	require.NoError(t, err)

	_, _, err = NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to decode header")
}

func TestSerialize_InvalidNodeRecords(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA256))

	h := &Header{
		NodeRecordCount:    1,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	_, _, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to decode node record")
}

func TestSerialize_TooManyNodeRecords(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &Header{
		NodeRecordCount:    10,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	_, _, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unexpected node record")
}

func TestSerialize_UnitDataConstructorError(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &Header{
		NodeRecordCount:    11,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	udc := func(_ types.UnitID) (types.UnitData, error) {
		return nil, fmt.Errorf("something happened")
	}
	_, _, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to construct unit data: something happened")
}

func TestSerialize_InvalidUnitDataConstructor(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &Header{
		NodeRecordCount:    11,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	udc := func(_ types.UnitID) (types.UnitData, error) {
		return struct{ *pruneUnitData }{}, nil
	}
	_, _, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to decode unit data")
}

func TestSerialize_InvalidChecksum(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &Header{
		NodeRecordCount:    11,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 1)

	_, _, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "checksum mismatch")
}

func TestSerialize_InvalidUC(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &Header{
		NodeRecordCount:    11,
		UnicityCertificate: createUC(t, s, 0, nil),
	}
	buf := createSerializedState(t, s, h, 0)

	_, _, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to commit recovered state")
}

func TestSerialize_EmptyStateUncommitted(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA256))

	buf := &bytes.Buffer{}
	require.NoError(t, s.Serialize(buf, true, nil))

	udc := func(_ types.UnitID) (types.UnitData, error) {
		return &pruneUnitData{}, nil
	}

	state, _, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)
	committed, err := state.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
}

func TestSerialize_EmptyStateCommitted(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA256))
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(t, s, summaryValue, summaryHash)))

	buf := &bytes.Buffer{}
	require.NoError(t, s.Serialize(buf, true, nil))

	udc := func(_ types.UnitID) (types.UnitData, error) {
		return &pruneUnitData{}, nil
	}

	state, _, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)
	committed, err := state.IsCommitted()
	require.NoError(t, err)
	require.True(t, committed)
}

func TestState_GetUnits(t *testing.T) {
	pdr := &types.PartitionDescriptionRecord{
		TypeIDLen: 8,
		UnitIDLen: 256,
	}
	unitID1 := append(make(types.UnitID, 31), 1, 1) // id=1 type=1
	unitID2 := append(make(types.UnitID, 31), 2, 1) // id=2 type=1
	unitID3 := append(make(types.UnitID, 31), 3, 1) // id=3 type=1
	unitID4 := append(make(types.UnitID, 31), 4, 2) // id=4 type=2
	unitID5 := append(make(types.UnitID, 31), 5, 2) // id=5 type=2
	s := NewEmptyState()
	require.NoError(t, s.Apply(
		AddUnit(unitID1, &TestData{Value: 1}),
		AddUnit(unitID2, &TestData{Value: 2}),
		AddUnit(unitID3, &TestData{Value: 3}),
		AddUnit(unitID4, &TestData{Value: 4}),
		AddUnit(unitID5, &TestData{Value: 5}),
	))
	// apply changes (get units works on the committed state)
	sum, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(15), sum)
	require.NotNil(t, rootHash)
	require.NoError(t, s.Commit(createUC(t, s, sum, rootHash)))

	t.Run("ok with no type id and no pdr", func(t *testing.T) {
		unitIDs, err := s.GetUnits(nil, nil)
		require.NoError(t, err)
		require.Len(t, unitIDs, 5)
	})
	t.Run("nok without pdr", func(t *testing.T) {
		typeID := uint32(1)
		unitIDs, err := s.GetUnits(&typeID, nil)
		require.ErrorContains(t, err, "partition description record is nil")
		require.Nil(t, unitIDs)
	})
	t.Run("nok with invalid pdr", func(t *testing.T) {
		typeID := uint32(3)
		unitIDs, err := s.GetUnits(&typeID, &types.PartitionDescriptionRecord{
			TypeIDLen: 16,
			UnitIDLen: 256,
		})
		require.ErrorContains(t, err, "failed to traverse state: extracting unit type from unit ID: expected unit ID length 34 bytes, got 33 bytes")
		require.Nil(t, unitIDs)
	})
	t.Run("ok with type id 1", func(t *testing.T) {
		typeID := uint32(1)
		unitIDs, err := s.GetUnits(&typeID, pdr)
		require.NoError(t, err)
		require.Len(t, unitIDs, 3)
		require.EqualValues(t, unitID1, unitIDs[0])
		require.EqualValues(t, unitID2, unitIDs[1])
		require.EqualValues(t, unitID3, unitIDs[2])
	})
	t.Run("ok with type id 2", func(t *testing.T) {
		typeID := uint32(2)
		unitIDs, err := s.GetUnits(&typeID, pdr)
		require.NoError(t, err)
		require.Len(t, unitIDs, 2)
		require.EqualValues(t, unitID4, unitIDs[0])
		require.EqualValues(t, unitID5, unitIDs[1])
	})
	t.Run("ok with type id 3", func(t *testing.T) {
		typeID := uint32(3)
		unitIDs, err := s.GetUnits(&typeID, pdr)
		require.NoError(t, err)
		require.Len(t, unitIDs, 0)
	})
}

func prepareState(t *testing.T) (*State, []byte, uint64) {
	s := NewEmptyState()
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
		AddUnit([]byte{0, 0, 0, 1}, &pruneUnitData{I: 10, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 6}, &pruneUnitData{I: 60, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 2}, &pruneUnitData{I: 20, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 3}, &pruneUnitData{I: 30, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 7}, &pruneUnitData{I: 70, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 4}, &pruneUnitData{I: 40, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 1, 0}, &pruneUnitData{I: 100, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 8}, &pruneUnitData{I: 80, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 5}, &pruneUnitData{I: 50, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 9}, &pruneUnitData{I: 90, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
		AddUnit([]byte{0, 0, 0, 0}, &pruneUnitData{I: 1, O: []byte{0x83, 0x00, 0x41, 0x01, 0xf6}}),
	))
	txrHash := test.RandomBytes(32)

	for _, id := range unitIdentifiers {
		require.NoError(t, s.AddUnitLog(id, txrHash))
	}

	sum, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(551), sum)
	require.NotNil(t, rootHash)
	require.NoError(t, s.Commit(createUC(t, s, sum, rootHash)))
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
		require.NoError(t, s.AddUnitLog(id, txrHash))
	}
	summaryValue, stateRootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NotNil(t, stateRootHash)

	require.NoError(t, s.Commit(createUC(t, s, summaryValue, stateRootHash)))
	return stateRootHash, summaryValue
}

func multiply(t uint64) func(data types.UnitData) (types.UnitData, error) {
	return func(data types.UnitData) (types.UnitData, error) {
		d := data.(*pruneUnitData)
		d.I = d.I * t
		return d, nil
	}
}

func getUnit(t *testing.T, s *State, id []byte) *UnitV1 {
	unit, err := s.latestSavepoint().Get(id)
	require.NoError(t, err)
	u, err := ToUnitV1(unit)
	require.NoError(t, err)
	return u
}

func createUC(t *testing.T, s *State, summaryValue uint64, summaryHash []byte) *types.UnicityCertificate {
	roundNumber := uint64(1)
	committed, err := s.IsCommitted()
	require.NoError(t, err)
	if committed {
		roundNumber = s.CommittedUC().GetRoundNumber() + 1
	}
	return &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
		Version:      1,
		RoundNumber:  roundNumber,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}
}

type pruneUnitData struct {
	_ struct{} `cbor:",toarray"`
	I uint64
	O []byte
}

func (p *pruneUnitData) Hash(hashAlgo crypto.Hash) ([]byte, error) {
	hasher := abhash.New(hashAlgo.New())
	p.Write(hasher)
	return hasher.Sum()
}

func (p *pruneUnitData) Write(hasher abhash.Hasher) {
	hasher.Write(p)
}

func (p *pruneUnitData) SummaryValueInput() uint64 {
	return p.I
}

func (p *pruneUnitData) Copy() types.UnitData {
	return &pruneUnitData{I: p.I}
}

func (p *pruneUnitData) Owner() []byte {
	return p.O
}

func (p *pruneUnitData) GetVersion() types.ABVersion {
	return 0
}

func unitDataConstructor(_ types.UnitID) (types.UnitData, error) {
	return &pruneUnitData{}, nil
}

func createSerializedState(t *testing.T, s *State, h *Header, checksum uint32) *bytes.Buffer {
	buf := &bytes.Buffer{}
	crc32Writer := NewCRC32Writer(buf)
	encoder, err := cbor.GetEncoder(crc32Writer)
	if err != nil {
		t.Fatal(err)
	}

	require.NoError(t, encoder.Encode(h))

	ss := newStateSerializer(encoder.Encode, s.hashAlgorithm)
	require.NoError(t, s.committedTree.Traverse(ss))

	if checksum == 0 {
		checksum = crc32Writer.Sum()
	}
	require.NoError(t, encoder.Encode(util.Uint32ToBytes(checksum)))
	return buf
}
