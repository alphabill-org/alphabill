package state

import (
	"bytes"
	"crypto"
	"fmt"
	"hash"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
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
	require.False(t, s.IsCommitted())
}

func TestNewStateWithSHA512(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA512))
	require.Equal(t, crypto.SHA512, s.hashAlgorithm)
}

func TestState_Savepoint_OK(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
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
	s := NewEmptyState()
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
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))

	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.False(t, s.isCommitted())
	require.Nil(t, s.CommittedUC())

	require.NoError(t, s.Commit(createUC(s, summaryValue, summaryHash)))
	require.True(t, s.isCommitted())
	require.NotNil(t, s.CommittedUC())
	require.EqualValues(t, 1, s.CommittedUC().GetRoundNumber())

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
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))

	require.ErrorContains(t, s.Commit(createUC(s, 0, nil)), "call CalculateRoot method before committing a state")
	require.False(t, s.isCommitted())
	require.Nil(t, s.CommittedUC())
}

func TestState_Commit_InvalidUC(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.ErrorContains(t, s.Commit(createUC(s, summaryValue, nil)), "state summary hash is not equal to the summary hash in UC")
	require.False(t, s.isCommitted())
	require.ErrorContains(t, s.Commit(createUC(s, 0, summaryHash)), "state summary value is not equal to the summary value in UC")
	require.False(t, s.isCommitted())
	require.Nil(t, s.CommittedUC())
}

func TestState_Apply_RevertsChangesAfterActionReturnsError(t *testing.T) {
	unitData := &TestData{Value: 10}
	s := NewEmptyState()
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
	s := NewEmptyState()
	require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), unitData)))
	s.Revert()

	committedRoot := s.committedTree.Root()
	uncommittedRoot := s.latestSavepoint().Root()
	require.Nil(t, committedRoot)
	require.Nil(t, uncommittedRoot)
	require.Len(t, s.savepoints, 1)
}

func TestState_NestedSavepointsCommitsAndReverts(t *testing.T) {
	s := NewEmptyState()
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
	require.NoError(t, s.Commit(createUC(s, summary, rootHash)))
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
	require.NoError(t, s.Commit(createUC(s, summary, rootHash2)))
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
	s := NewEmptyState()
	id := s.Savepoint()
	require.NoError(t,
		s.Apply(AddUnit([]byte{0, 0, 0, 0}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 2}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 3}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 4}, test.RandomBytes(20), &TestData{Value: 1})),
	)
	s.ReleaseToSavepoint(id)
	value, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(s, value, hash)))
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
	summary, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(s, summary, hash)))
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
		s.Apply(AddUnit([]byte{0, 0, 0, 20}, test.RandomBytes(20), &TestData{Value: 20})),
		s.Apply(AddUnit([]byte{0, 0, 0, 10}, test.RandomBytes(20), &TestData{Value: 10})),
		s.Apply(AddUnit([]byte{0, 0, 0, 30}, test.RandomBytes(20), &TestData{Value: 30})),
		s.Apply(AddUnit([]byte{0, 0, 0, 1}, test.RandomBytes(20), &TestData{Value: 1})),
		s.Apply(AddUnit([]byte{0, 0, 0, 15}, test.RandomBytes(20), &TestData{Value: 15})),
	)

	// commit initial state
	value, root, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(s, value, root)))

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
	s := NewEmptyState()

	require.NoError(t, s.Apply(AddUnit(unitID, test.RandomBytes(20), unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))

	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	u, err = s.GetUnit(unitID, true)
	require.ErrorContains(t, err, "item 00000001 does not exist")
	require.Nil(t, u)

	value, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(s, value, hash)))

	u, err = s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	u2, err := s.GetUnit(unitID, true)
	require.NoError(t, err)
	require.NotNil(t, u2)
	// logRoot, subTreeSummaryHash and summaryCalculated do not get cloned - rest must match
	require.Equal(t, u.logs, u2.logs)
	require.Equal(t, u.bearer, u2.bearer)
	require.Equal(t, u.data, u2.data)
	require.Equal(t, u.subTreeSummaryValue, u2.subTreeSummaryValue)
}

func TestState_AddUnitLog_OK(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewEmptyState()

	require.NoError(t, s.Apply(AddUnit(unitID, test.RandomBytes(20), unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))
	txrHash := test.RandomBytes(32)
	require.NoError(t, s.AddUnitLog(unitID, txrHash))

	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.Len(t, u.logs, 2)
	require.Equal(t, txrHash, u.logs[1].TxRecordHash)
}

func TestState_CommitTreeWithLeftAndRightChildNodes(t *testing.T) {
	s := NewEmptyState()
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
	s := NewEmptyState()
	require.ErrorContains(t, s.AddUnitLog(unitID, test.RandomBytes(32)), "unable to add unit log for unit 00000001")
}

func TestState_PruneState(t *testing.T) {
	unitID := []byte{0, 0, 0, 1}
	unitData := &TestData{Value: 10}
	s := NewEmptyState()

	require.NoError(t, s.Apply(AddUnit(unitID, test.RandomBytes(20), unitData)))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))

	require.NoError(t, s.Apply(UpdateUnitData(unitID, func(data UnitData) (UnitData, error) {
		data.(*TestData).Value = 100
		return data, nil
	})))
	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))
	u, err := s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.Len(t, u.logs, 2)
	require.NoError(t, s.Prune())
	u, err = s.GetUnit(unitID, false)
	require.NoError(t, err)
	require.Len(t, u.logs, 1)
	require.Nil(t, u.logs[0].TxRecordHash)
}

func TestCreateAndVerifyStateProofs_CreateUnits(t *testing.T) {
	s, stateRootHash, summaryValue := prepareState(t)
	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0)
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
		stateProof, err := s.CreateUnitStateProof(id, 0)
		require.NoError(t, err)

		proofOutputHash, sum := stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)

		stateProof, err = s.CreateUnitStateProof(id, 1)
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
	require.NoError(t, s.Prune())
	value, hash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(s, value, hash)))
	stateRootHash, summaryValue := updateUnits(t, s)
	require.Equal(t, uint64(55100), summaryValue)

	for _, id := range unitIdentifiers {
		stateProof, err := s.CreateUnitStateProof(id, 0)
		require.NoError(t, err)

		proofOutputHash, sum := stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)

		stateProof, err = s.CreateUnitStateProof(id, 1)
		require.NoError(t, err)

		proofOutputHash, sum = stateProof.CalculateSateTreeOutput(s.hashAlgorithm)
		require.Equal(t, summaryValue, sum, "invalid summary value for unit %v: got %d, expected %d", id, sum, summaryValue)
		require.Equal(t, stateRootHash, proofOutputHash, "invalid chain output hash for unit %v: got %X, expected %X", id, proofOutputHash, stateRootHash)
	}
}

type alwaysValid struct{}

func (a *alwaysValid) Validate(*types.UnicityCertificate) error {
	return nil
}

func TestCreateAndVerifyStateProofs_CreateUnitProof(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s, _, _ := prepareState(t)
		proof, err := s.CreateUnitStateProof([]byte{0, 0, 0, 5}, 0)
		require.NoError(t, err)
		unit, err := s.GetUnit([]byte{0, 0, 0, 5}, true)
		require.NoError(t, err)
		unitData, err := MarshalUnitData(unit.Data())
		require.NoError(t, err)
		data := &types.StateUnitData{
			Data:   unitData,
			Bearer: unit.Bearer(),
		}
		require.NoError(t, types.VerifyUnitStateProof(proof, crypto.SHA256, data, &alwaysValid{}))
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
	uc := createUC(s, summaryValue, summaryHash)
	require.NoError(t, s.Commit(uc))

	buf := &bytes.Buffer{}
	// Writes the pruned state
	require.NoError(t, s.Serialize(buf, true))

	recoveredState, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)

	recoveredSummaryValue, recoveredSummaryHash, err := recoveredState.CalculateRoot()
	require.NoError(t, err)
	require.True(t, recoveredState.IsCommitted())
	require.Equal(t, summaryValue, recoveredSummaryValue)
	require.Equal(t, summaryHash, recoveredSummaryHash)
	require.Equal(t, uc, recoveredState.CommittedUC())
}

func TestSerialize_InvalidHeader(t *testing.T) {
	s, _, _ := prepareState(t)

	buf := &bytes.Buffer{}
	// Writes the pruned state
	require.NoError(t, s.Serialize(buf, true))

	_, err := buf.ReadByte()
	require.NoError(t, err)

	_, err = NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to decode header")
}

func TestSerialize_InvalidNodeRecords(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA256))

	h := &header{
		NodeRecordCount:    1,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	_, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to decode node record")
}

func TestSerialize_TooManyNodeRecords(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &header{
		NodeRecordCount:    10,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	_, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unexpected node record")
}

func TestSerialize_UnitDataConstructorError(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &header{
		NodeRecordCount:    11,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	udc := func(_ types.UnitID) (UnitData, error) {
		return nil, fmt.Errorf("something happened")
	}
	_, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to construct unit data: something happened")
}

func TestSerialize_InvalidUnitDataConstructor(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &header{
		NodeRecordCount:    11,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 0)

	udc := func(_ types.UnitID) (UnitData, error) {
		return struct{ *pruneUnitData }{}, nil
	}
	_, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to decode unit data")
}

func TestSerialize_InvalidChecksum(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &header{
		NodeRecordCount:    11,
		UnicityCertificate: nil,
	}
	buf := createSerializedState(t, s, h, 1)

	_, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "checksum mismatch")
}

func TestSerialize_InvalidUC(t *testing.T) {
	s, _, _ := prepareState(t)

	h := &header{
		NodeRecordCount:    11,
		UnicityCertificate: createUC(s, 0, nil),
	}
	buf := createSerializedState(t, s, h, 0)

	_, err := NewRecoveredState(buf, unitDataConstructor, WithHashAlgorithm(crypto.SHA256))
	require.ErrorContains(t, err, "unable to commit recovered state")
}

func TestSerialize_EmptyStateUncommitted(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA256))

	buf := &bytes.Buffer{}
	require.NoError(t, s.Serialize(buf, true))

	udc := func(_ types.UnitID) (UnitData, error) {
		return &pruneUnitData{}, nil
	}

	state, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)
	require.False(t, state.IsCommitted())
}

func TestSerialize_EmptyStateCommitted(t *testing.T) {
	s := NewEmptyState(WithHashAlgorithm(crypto.SHA256))
	summaryValue, summaryHash, err := s.CalculateRoot()
	if summaryHash == nil {
		summaryHash = make([]byte, crypto.SHA256.Size())
	}
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(s, summaryValue, summaryHash)))

	buf := &bytes.Buffer{}
	require.NoError(t, s.Serialize(buf, true))

	udc := func(_ types.UnitID) (UnitData, error) {
		return &pruneUnitData{}, nil
	}

	state, err := NewRecoveredState(buf, udc, WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)
	require.True(t, state.IsCommitted())
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
		AddUnit([]byte{0, 0, 0, 1}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 10}),
		AddUnit([]byte{0, 0, 0, 6}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 60}),
		AddUnit([]byte{0, 0, 0, 2}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 20}),
		AddUnit([]byte{0, 0, 0, 3}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 30}),
		AddUnit([]byte{0, 0, 0, 7}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 70}),
		AddUnit([]byte{0, 0, 0, 4}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 40}),
		AddUnit([]byte{0, 0, 1, 0}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 100}),
		AddUnit([]byte{0, 0, 0, 8}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 80}),
		AddUnit([]byte{0, 0, 0, 5}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 50}),
		AddUnit([]byte{0, 0, 0, 9}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 90}),
		AddUnit([]byte{0, 0, 0, 0}, []byte{0x83, 0x00, 0x41, 0x01, 0xf6}, &pruneUnitData{I: 1}),
	))
	txrHash := test.RandomBytes(32)

	for _, id := range unitIdentifiers {
		require.NoError(t, s.AddUnitLog(id, txrHash))
	}

	sum, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(551), sum)
	require.NotNil(t, rootHash)
	require.NoError(t, s.Commit(createUC(s, sum, rootHash)))
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

	require.NoError(t, s.Commit(createUC(s, summaryValue, stateRootHash)))
	return stateRootHash, summaryValue
}

func multiply(t uint64) func(data UnitData) (UnitData, error) {
	return func(data UnitData) (UnitData, error) {
		d := data.(*pruneUnitData)
		d.I = d.I * t
		return d, nil
	}
}

func getUnit(t *testing.T, s *State, id []byte) *Unit {
	unit, err := s.latestSavepoint().Get(id)
	require.NoError(t, err)
	return unit
}

func createUC(s *State, summaryValue uint64, summaryHash []byte) *types.UnicityCertificate {
	roundNumber := uint64(1)
	if s.IsCommitted() {
		roundNumber = s.CommittedUC().GetRoundNumber() + 1
	}
	return &types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  roundNumber,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}
}

type pruneUnitData struct {
	_ struct{} `cbor:",toarray"`
	I uint64
}

func (p *pruneUnitData) Hash(hashAlgo crypto.Hash) []byte {
	hasher := hashAlgo.New()
	_ = p.Write(hasher)
	return hasher.Sum(nil)
}

func (p *pruneUnitData) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(p)
	if err != nil {
		return fmt.Errorf("unit data encode error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (p *pruneUnitData) SummaryValueInput() uint64 {
	return p.I
}

func (p *pruneUnitData) Copy() UnitData {
	return &pruneUnitData{I: p.I}
}

func unitDataConstructor(_ types.UnitID) (UnitData, error) {
	return &pruneUnitData{}, nil
}

func createSerializedState(t *testing.T, s *State, h *header, checksum uint32) *bytes.Buffer {
	buf := &bytes.Buffer{}
	crc32Writer := NewCRC32Writer(buf)
	encoder := cbor.NewEncoder(crc32Writer)

	require.NoError(t, encoder.Encode(h))

	ss := newStateSerializer(encoder, s.hashAlgorithm)
	s.committedTree.Traverse(ss)
	require.NoError(t, ss.err)

	if checksum == 0 {
		checksum = crc32Writer.Sum()
	}
	require.NoError(t, encoder.Encode(util.Uint32ToBytes(checksum)))
	return buf
}
