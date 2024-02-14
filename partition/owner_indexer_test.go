package partition

import (
	"hash"
	"testing"

	"github.com/stretchr/testify/require"

	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

func TestOwnerIndexer(t *testing.T) {
	t.Run("last owner of unit is added to index", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		pubKeyHash1 := []byte{1}
		pubKeyHash2 := []byte{2}
		pubKeyHash3 := []byte{3}
		logs := []*state.Log{
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(pubKeyHash1)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(pubKeyHash2)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(pubKeyHash3)},
		}
		err := ownerIndexer.IndexOwner(unitID, logs)
		require.NoError(t, err)

		// verify that owner index contains only the last owner
		require.Len(t, ownerIndexer.ownerUnits, 1)
		ownerUnitIDs := ownerIndexer.ownerUnits[string(pubKeyHash3)]
		require.Len(t, ownerUnitIDs, 1)
		require.Equal(t, unitID, ownerUnitIDs[0])
	})
	t.Run("unit is removed from previous owner index (single unit is deleted)", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID1 := types.UnitID{1}
		unitID2 := types.UnitID{2}
		ownerID1 := []byte{1}
		ownerID2 := []byte{2}

		// set index owner1 => [unit1, unit2]
		ownerIndexer.ownerUnits[string(ownerID1)] = []types.UnitID{unitID1, unitID2}

		// transfer unit2 to owner2
		logs := []*state.Log{
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID1)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID2)},
		}
		err := ownerIndexer.IndexOwner(unitID2, logs)
		require.NoError(t, err)

		// verify that unit2 is removed from owner 1
		owner1UnitIDs := ownerIndexer.ownerUnits[string(ownerID1)]
		require.Len(t, owner1UnitIDs, 1)
		require.Equal(t, unitID1, owner1UnitIDs[0])
	})
	t.Run("unit is removed from previous owner index (entire cache entry is deleted)", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID1 := []byte{1}
		ownerID2 := []byte{2}

		// set index owner1 => [unit]
		ownerIndexer.ownerUnits[string(ownerID1)] = []types.UnitID{unitID}

		// transfer unit to owner2
		logs := []*state.Log{
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID1)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID2)},
		}
		err := ownerIndexer.IndexOwner(unitID, logs)
		require.NoError(t, err)

		// verify that unit1 index is removed
		owner1UnitIDs, ok := ownerIndexer.ownerUnits[string(ownerID1)]
		require.False(t, ok)
		require.Nil(t, owner1UnitIDs)
	})
	t.Run("non-p2pkh predicate is not indexed", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID := templates.AlwaysTrueBytes()
		logs := []*state.Log{{NewBearer: ownerID}}
		err := ownerIndexer.IndexOwner(unitID, logs)
		require.NoError(t, err)

		// verify that unit is indexed
		ownerUnitIDs := ownerIndexer.ownerUnits[string(ownerID)]
		require.Len(t, ownerUnitIDs, 0)
	})
	t.Run("index can be loaded from state", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID := []byte{1}
		ownerPredicate := templates.NewP2pkh256BytesFromKeyHash(ownerID)
		s := state.NewEmptyState()
		err := s.Apply(state.AddUnit(unitID, ownerPredicate, &mockUnitData{}))
		require.NoError(t, err)
		summaryValue, summaryHash, err := s.CalculateRoot()
		require.NoError(t, err)
		err = s.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
			RoundNumber:  1,
			Hash:         summaryHash,
			SummaryValue: util.Uint64ToBytes(summaryValue),
		}})
		require.NoError(t, err)

		err = ownerIndexer.LoadState(s)
		require.NoError(t, err)

		// verify that unit is indexed
		ownerUnitIDs, err := ownerIndexer.GetOwnerUnits(ownerID)
		require.NoError(t, err)
		require.Len(t, ownerUnitIDs, 1)
		require.Equal(t, unitID, ownerUnitIDs[0])
	})
}

type mockUnitData struct{}

func (m mockUnitData) Write(hash.Hash) error { return nil }

func (m mockUnitData) SummaryValueInput() uint64 {
	return 0
}

func (m mockUnitData) Copy() state.UnitData {
	return mockUnitData{}
}
