package state

import (
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var (
	unitID1 = []byte{0, 0, 0, 1}
	unitID2 = []byte{0, 0, 0, 2}
	unitID3 = []byte{0, 0, 0, 3}

	units = []types.UnitID{unitID3, unitID2, unitID1}
)

func TestStatePruning_Count(t *testing.T) {
	s, _ := createStateWithUnits(t)
	p := NewLogPruner(s)
	p.Add(1, unitID2)
	p.Add(1, unitID3)

	require.Equal(t, 0, p.Count(0))
	require.Equal(t, 2, p.Count(1))
}

func TestStatePruning_Remove(t *testing.T) {
	s, _ := createStateWithUnits(t)
	p := NewLogPruner(s)
	p.Add(1, unitID2)
	p.Add(1, unitID3)
	p.Remove(1)
	require.Equal(t, 0, p.Count(0))
	require.Equal(t, 0, p.Count(1))
}

func TestStatePruning_Prune(t *testing.T) {
	s, _ := createStateWithUnits(t)

	unit2, err := s.GetUnit(unitID2, true)
	require.NoError(t, err)
	require.Equal(t, 2, len(unit2.logs))
	unit3, err := s.GetUnit(unitID3, true)
	require.NoError(t, err)
	require.Equal(t, 2, len(unit3.logs))

	p := NewLogPruner(s)
	p.Add(1, unitID2)
	p.Add(1, unitID3)
	require.NoError(t, p.Prune(1))
	prunedUnit2, err := s.GetUnit(unitID2, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(prunedUnit2.logs))
	prunedUnit3, err := s.GetUnit(unitID3, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(prunedUnit3.logs))
}

func TestStatePruning_RevertPrune(t *testing.T) {
	s, rootHash := createStateWithUnits(t)
	unit2, err := s.GetUnit(unitID2, true)
	require.NoError(t, err)
	require.Equal(t, 2, len(unit2.logs))
	unit3, err := s.GetUnit(unitID3, true)
	require.NoError(t, err)
	require.Equal(t, 2, len(unit3.logs))
	p := NewLogPruner(s)
	p.Add(1, unitID2)
	p.Add(1, unitID3)
	require.NoError(t, p.Prune(1))
	prunedUnit2, err := s.GetUnit(unitID2, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(prunedUnit2.logs))
	prunedUnit3, err := s.GetUnit(unitID3, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(prunedUnit3.logs))

	require.NoError(t, s.Apply(UpdateUnitData(unitID2, func(data UnitData) (UnitData, error) {
		data.(*pruneUnitData).i = data.(*pruneUnitData).i * 10
		return data, nil
	},
	), UpdateUnitData(unitID3, func(data UnitData) (UnitData, error) {
		data.(*pruneUnitData).i = data.(*pruneUnitData).i * 10
		return data, nil
	})))

	s.Revert()

	_, rootHash2, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, rootHash, rootHash2)
	require.NoError(t, s.Commit())
	_, rootHash3, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, rootHash, rootHash3)
	unit2, err = s.GetUnit(unitID2, true)
	require.NoError(t, err)
	require.Equal(t, 2, len(unit2.logs))
	unit3, err = s.GetUnit(unitID3, true)
	require.NoError(t, err)
	require.Equal(t, 2, len(unit3.logs))
}

type pruneUnitData struct {
	i uint64
}

func (p *pruneUnitData) Write(hasher hash.Hash) error {
	hasher.Write(util.Uint64ToBytes(p.i))
	return nil
}

func (p *pruneUnitData) SummaryValueInput() uint64 {
	return p.i
}

func (p *pruneUnitData) Copy() UnitData {
	return &pruneUnitData{i: p.i}
}

func createStateWithUnits(t *testing.T) (*State, []byte) {
	s := NewEmptyState()
	require.NoError(t, s.Apply(
		AddUnit(unitID1, templates.AlwaysTrueBytes(), &pruneUnitData{i: 1}),
		AddUnit(unitID2, templates.AlwaysTrueBytes(), &pruneUnitData{i: 2}),
		AddUnit(unitID3, templates.AlwaysTrueBytes(), &pruneUnitData{i: 3}),
	))

	for _, id := range units {
		_, err := s.AddUnitLog(id, test.RandomBytes(32))
		require.NoError(t, err)
	}

	require.NoError(t, s.Apply(
		UpdateUnitData(unitID2, func(data UnitData) (UnitData, error) {
			return &pruneUnitData{i: 22}, nil
		}),
		UpdateUnitData(unitID3, func(data UnitData) (UnitData, error) {
			return &pruneUnitData{i: 32}, nil
		}),
	))

	_, err := s.AddUnitLog(unitID2, test.RandomBytes(32))
	require.NoError(t, err)
	_, err = s.AddUnitLog(unitID3, test.RandomBytes(32))

	require.NoError(t, err)
	summary, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, uint64(55), summary)
	require.NoError(t, s.Commit())
	return s, rootHash
}
