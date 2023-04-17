package monolithic

import (
	gocrypto "crypto"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var zeroHash = make([]byte, gocrypto.SHA256.Size())
var mockUc = &certificates.UnicityCertificate{
	InputRecord: &certificates.InputRecord{
		RoundNumber:  1,
		Hash:         zeroHash,
		PreviousHash: zeroHash,
		BlockHash:    zeroHash,
		SummaryValue: []byte{0, 0, 0, 0},
	},
	UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
		SystemIdentifier:      sysID0,
		SiblingHashes:         nil,
		SystemDescriptionHash: nil,
	},
	UnicitySeal: &certificates.UnicitySeal{
		RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 1},
		CommitInfo:    &certificates.CommitInfo{RootHash: make([]byte, gocrypto.SHA256.Size())},
	},
}

var testGenesis = &genesis.RootGenesis{
	Partitions: []*genesis.GenesisPartitionRecord{
		{
			Nodes:       nil,
			Certificate: mockUc,
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: sysID0,
				T2Timeout:        2500,
			},
		},
		{
			Nodes:       nil,
			Certificate: mockUc,
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: sysID1,
				T2Timeout:        2500,
			},
		},
	},
}

func storeTest(t *testing.T, store *StateStore) {
	empty, err := store.IsEmpty()
	require.NoError(t, err)
	require.True(t, empty)
	// read round from empty
	round, err := store.GetRound()
	require.ErrorContains(t, err, "round not stored in db")
	require.Equal(t, uint64(0), round)
	require.Error(t, store.Init(nil))
	// Update genesis state
	require.NoError(t, store.Init(testGenesis))
	lastCert, err := store.GetCertificate(protocol.SystemIdentifier(sysID0))
	require.NoError(t, err)
	require.True(t, proto.Equal(lastCert, mockUc))
	round, err = store.GetRound()
	require.NoError(t, err)
	require.Equal(t, uint64(1), round)
	// Update only round
	require.NoError(t, store.Update(2, nil))
	round, err = store.GetRound()
	require.NoError(t, err)
	require.Equal(t, uint64(2), round)
	// Root round number can skip rounds, but must not be smaller or equal
	require.Error(t, store.Update(1, nil))
	require.Error(t, store.Update(2, nil))
	newUC := proto.Clone(mockUc).(*certificates.UnicityCertificate)
	newUC.InputRecord = &certificates.InputRecord{
		RoundNumber:  2,
		Hash:         []byte{1, 2, 3},
		PreviousHash: []byte{3, 2, 1},
		SummaryValue: []byte{2, 4, 6, 8},
		BlockHash:    []byte{3, 3, 3},
	}
	update := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{
		protocol.SystemIdentifier(sysID0): newUC,
	}
	require.NoError(t, store.Update(3, update))
	lastCert, err = store.GetCertificate(protocol.SystemIdentifier(sysID0))
	require.NoError(t, err)
	require.True(t, proto.Equal(lastCert, newUC))
	IRmap, err := store.GetLastCertifiedInputRecords()
	require.NoError(t, err)
	require.Contains(t, IRmap, protocol.SystemIdentifier(sysID0))
	ir := IRmap[protocol.SystemIdentifier(sysID0)]
	require.True(t, proto.Equal(ir, mockUc.InputRecord))
	// read non-existing system id
	lastCert, err = store.GetCertificate(protocol.SystemIdentifier(sysID2))
	require.ErrorContains(t, err, "certificate id 00000002 not found")
	require.Nil(t, lastCert)
	// read sys id 1
	lastCert, err = store.GetCertificate(protocol.SystemIdentifier(sysID1))
	require.NoError(t, err)
	require.True(t, proto.Equal(lastCert, mockUc))
}

func TestInMemState(t *testing.T) {
	stateStore := NewStateStore(memorydb.New())
	require.NotNil(t, stateStore)
	storeTest(t, stateStore)
}

func TestPersistentRootState(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	dir := t.TempDir()
	db, err := boltdb.New(filepath.Join(dir, "bolt.db"))
	require.NoError(t, err)
	stateStore := NewStateStore(db)
	storeTest(t, stateStore)
}

func TestRepeatedStore(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	dir := t.TempDir()
	db, err := boltdb.New(filepath.Join(dir, "bolt.db"))
	require.NoError(t, err)
	store := NewStateStore(db)
	storeTest(t, store)
	require.NoError(t, store.Init(testGenesis))
	require.NoError(t, store.Update(2, nil))
	require.NoError(t, store.Update(3, nil))
	require.NoError(t, store.Update(4, nil))
	require.NoError(t, store.Update(5, nil))
	require.NoError(t, store.Update(6, nil))
	require.NoError(t, store.Update(7, nil))
	require.NoError(t, store.Update(8, nil))
}
