package event

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/state"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

type mockStateStoreOK struct{}

func TestNewProofIndexer(t *testing.T) {
	t.Run("indexer - history size 2", func(t *testing.T) {
		proofDB := memorydb.New()
		indexer := NewProofIndexer(proofDB, 2, testlogger.New(t))
		ev := createEvent(1, 10)
		indexer.Handle(ev)
		ev = createEvent(2, 10)
		indexer.Handle(ev)
		ev = createEvent(3, 10)
		indexer.Handle(ev)
		ev = createEvent(4, 10)
		indexer.Handle(ev)
		ev = createEvent(5, 10)
		indexer.Handle(ev)
		require.EqualValues(t, 5, indexer.latestIndexedBlockNumber())
	})
}

func (m mockStateStoreOK) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return &state.Unit{}, nil
}

func (m mockStateStoreOK) CreateUnitStateProof(id types.UnitID, logIndex int, uc *types.UnicityCertificate) (*types.UnitStateProof, error) {
	return &types.UnitStateProof{}, nil
}

func createEvent(round uint64, unitID uint64) *Event {
	block := &types.Block{
		Header: &types.Header{SystemID: []byte{0, 0, 0, 1}},
		Transactions: []*types.TransactionRecord{
			{
				TransactionOrder: &types.TransactionOrder{},
				ServerMetadata:   &types.ServerMetadata{TargetUnits: []types.UnitID{util.Uint64ToBytes(unitID)}},
			},
		},
		UnicityCertificate: &types.UnicityCertificate{
			InputRecord: &types.InputRecord{RoundNumber: round},
		},
	}
	return &Event{
		EventType: BlockFinalized,
		Content: &struct {
			Block *types.Block
			State txsystem.UnitAndProof
		}{
			Block: block,
			State: mockStateStoreOK{},
		},
	}
}
