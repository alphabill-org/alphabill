package partition

import (
	"bytes"
	"hash"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

func TestOwnerIndexer(t *testing.T) {
	t.Run("last owner of unit is added to index", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))

		// create initial state
		s := state.NewEmptyState()
		unitID := types.UnitID{1}
		initialOwner := templates.NewP2pkh256BytesFromKeyHash([]byte{0})
		unitData := &mockUnitData{ownerPredicate: initialOwner}
		err := s.Apply(state.AddUnit(unitID, unitData))
		require.NoError(t, err)

		// add units to state
		for i := byte(1); i <= 3; i++ {
			ownerID := templates.NewP2pkh256BytesFromKeyHash([]byte{i})
			err = s.Apply(state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
				unitData.ownerPredicate = ownerID
				return unitData, nil
			}))
			require.NoError(t, err)

			err = s.AddUnitLog(unitID, []byte{i})
			require.NoError(t, err)
		}
		commitState(t, s)

		// update index with given state and block
		b := &types.Block{Transactions: []*types.TransactionRecord{testtransaction.NewTransactionRecord(t, testtransaction.WithUnitID(unitID))}}
		err = ownerIndexer.IndexBlock(b, s)
		require.NoError(t, err)

		// verify that owner index contains the last owner
		require.Len(t, ownerIndexer.ownerUnits, 1)
		ownerUnitIDs := ownerIndexer.ownerUnits[string([]byte{3})]
		require.Len(t, ownerUnitIDs, 1)
		require.Equal(t, unitID, ownerUnitIDs[0])
	})
	t.Run("unit is removed from previous owner index (single entry is deleted)", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID1 := types.UnitID{1}
		unitID2 := types.UnitID{2}
		ownerID1 := []byte{1}
		ownerID2 := []byte{2}
		owner1Predicate := templates.NewP2pkh256BytesFromKeyHash(ownerID1)
		owner2Predicate := templates.NewP2pkh256BytesFromKeyHash(ownerID2)

		// set index owner1 owns both units
		ownerIndexer.ownerUnits[string(ownerID1)] = []types.UnitID{unitID1, unitID2}

		// create state where unit2 owner was changed owner1->owner2
		s := state.NewEmptyState()
		unitData := &mockUnitData{ownerPredicate: owner1Predicate}
		require.NoError(t, s.Apply(state.AddUnit(unitID2, unitData)))
		require.NoError(t, s.AddUnitLog(unitID2, test.RandomBytes(4)))
		require.NoError(t, s.Apply(state.UpdateUnitData(unitID2, func(data types.UnitData) (types.UnitData, error) {
			unitData.ownerPredicate = owner2Predicate
			return unitData, nil
		})))
		require.NoError(t, s.AddUnitLog(unitID2, test.RandomBytes(4)))
		commitState(t, s)

		// update index
		b := &types.Block{Transactions: []*types.TransactionRecord{
			testtransaction.NewTransactionRecord(t, testtransaction.WithUnitID(unitID2)),
		}}
		require.NoError(t, ownerIndexer.IndexBlock(b, s))

		// verify that unit2 is removed from owner1
		owner1UnitIDs := ownerIndexer.ownerUnits[string(ownerID1)]
		require.Len(t, owner1UnitIDs, 1)
		require.Equal(t, unitID1, owner1UnitIDs[0])

		// and unit2 is added to owner2
		owner2UnitIDs := ownerIndexer.ownerUnits[string(ownerID2)]
		require.Len(t, owner2UnitIDs, 1)
		require.Equal(t, unitID2, owner2UnitIDs[0])
	})
	t.Run("unit is removed from previous owner index (entire cache entry is deleted)", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID1 := []byte{1}
		ownerID2 := []byte{2}
		owner1Predicate := templates.NewP2pkh256BytesFromKeyHash(ownerID1)
		owner2Predicate := templates.NewP2pkh256BytesFromKeyHash(ownerID2)

		// set index owner1 => [unit]
		ownerIndexer.ownerUnits[string(ownerID1)] = []types.UnitID{unitID}

		// create state where unit was transferred to owner2
		s := state.NewEmptyState()
		unitData := &mockUnitData{ownerPredicate: owner1Predicate}
		require.NoError(t, s.Apply(state.AddUnit(unitID, unitData)))
		require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(4)))
		require.NoError(t, s.Apply(state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
			unitData.ownerPredicate = owner2Predicate
			return unitData, nil
		})))
		require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(4)))
		commitState(t, s)

		// update index
		b := &types.Block{Transactions: []*types.TransactionRecord{
			testtransaction.NewTransactionRecord(t, testtransaction.WithUnitID(unitID)),
		}}
		require.NoError(t, ownerIndexer.IndexBlock(b, s))

		// verify that owner1 index is removed
		owner1UnitIDs, ok := ownerIndexer.ownerUnits[string(ownerID1)]
		require.False(t, ok)
		require.Nil(t, owner1UnitIDs)

		// and owner2 index is updated
		owner2UnitIDs, ok := ownerIndexer.ownerUnits[string(ownerID2)]
		require.True(t, ok)
		require.Len(t, owner2UnitIDs, 1)
		require.Equal(t, owner2UnitIDs[0], unitID)
	})
	t.Run("random owner bytes are not indexed", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		ownerPredicate := []byte{123}

		// create state with random bytes for owner predicate
		s := state.NewEmptyState()
		require.NoError(t, s.Apply(state.AddUnit(unitID, &mockUnitData{})))
		require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(4)))
		commitState(t, s)

		// update index
		b := &types.Block{Transactions: []*types.TransactionRecord{
			testtransaction.NewTransactionRecord(t, testtransaction.WithUnitID(unitID)),
		}}
		require.NoError(t, ownerIndexer.IndexBlock(b, s))

		// verify that unit is not indexed
		ownerUnitIDs := ownerIndexer.ownerUnits[string(ownerPredicate)]
		require.Len(t, ownerUnitIDs, 0)
	})
	t.Run("non-p2pkh predicate is not indexed", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID := templates.AlwaysTrueBytes()

		// create state with alwaysTrue unit
		s := state.NewEmptyState()
		require.NoError(t, s.Apply(state.AddUnit(unitID, &mockUnitData{})))
		require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(4)))
		commitState(t, s)

		// update index
		b := &types.Block{Transactions: []*types.TransactionRecord{
			testtransaction.NewTransactionRecord(t, testtransaction.WithUnitID(unitID)),
		}}
		require.NoError(t, ownerIndexer.IndexBlock(b, s))

		// verify that unit is not indexed
		ownerUnitIDs := ownerIndexer.ownerUnits[string(ownerID)]
		require.Len(t, ownerUnitIDs, 0)
	})
	t.Run("index can be loaded from state", func(t *testing.T) {
		ownerIndexer := NewOwnerIndexer(testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID := []byte{1}
		ownerPredicate := templates.NewP2pkh256BytesFromKeyHash(ownerID)
		s := state.NewEmptyState()
		require.NoError(t, s.Apply(state.AddUnit(unitID, &mockUnitData{ownerPredicate: ownerPredicate})))
		require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(4)))
		commitState(t, s)

		// load state
		require.NoError(t, ownerIndexer.LoadState(s))

		// verify that unit is indexed
		ownerUnitIDs, err := ownerIndexer.GetOwnerUnits(ownerID)
		require.NoError(t, err)
		require.Len(t, ownerUnitIDs, 1)
		require.Equal(t, unitID, ownerUnitIDs[0])
	})
}

type mockUnitData struct {
	ownerPredicate []byte
}

func (m mockUnitData) Write(hash.Hash) error { return nil }

func (m mockUnitData) SummaryValueInput() uint64 {
	return 0
}

func (m mockUnitData) Copy() types.UnitData {
	return mockUnitData{ownerPredicate: bytes.Clone(m.ownerPredicate)}
}

func (m mockUnitData) Owner() []byte {
	return m.ownerPredicate
}

func commitState(t *testing.T, s *state.State) {
	rootVal, rootHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(createUC(s, rootVal, rootHash)))
}

func createUC(s *state.State, summaryValue uint64, summaryHash []byte) *types.UnicityCertificate {
	roundNumber := uint64(1)
	if s.IsCommitted() {
		roundNumber = s.CommittedUC().GetRoundNumber() + 1
	}
	return &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{Version: 1,
		RoundNumber:  roundNumber,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}
}
