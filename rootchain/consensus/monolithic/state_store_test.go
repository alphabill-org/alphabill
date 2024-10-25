package monolithic

import (
	gocrypto "crypto"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, gocrypto.SHA256.Size())
var mockUc = &types.UnicityCertificate{Version: 1,
	InputRecord: &types.InputRecord{Version: 1,
		RoundNumber:  1,
		Hash:         zeroHash,
		PreviousHash: zeroHash,
		BlockHash:    zeroHash,
		SummaryValue: []byte{0, 0, 0, 0},
	},
	UnicityTreeCertificate: &types.UnicityTreeCertificate{Version: 1,
		SystemIdentifier:         sysID3,
		HashSteps:                nil,
		PartitionDescriptionHash: nil,
	},
	UnicitySeal: &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: 1,
		Hash:                 make([]byte, gocrypto.SHA256.Size()),
		Signatures:           types.SignatureMap{},
	},
}

var testGenesis = &genesis.RootGenesis{Version: 1,
	Partitions: []*genesis.GenesisPartitionRecord{
		{
			Version:     1,
			Nodes:       nil,
			Certificate: mockUc,
			PartitionDescription: &types.PartitionDescriptionRecord{
				Version:           1,
				NetworkIdentifier: 5,
				SystemIdentifier:  sysID3,
				T2Timeout:         2500 * time.Millisecond,
			},
		},
		{
			Version:     1,
			Nodes:       nil,
			Certificate: mockUc,
			PartitionDescription: &types.PartitionDescriptionRecord{
				Version:           1,
				NetworkIdentifier: 5,
				SystemIdentifier:  sysID1,
				T2Timeout:         2500 * time.Millisecond,
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
	lastCert, err := store.GetCertificate(sysID3)
	require.NoError(t, err)
	require.Equal(t, mockUc, &lastCert.UC)
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

	newUC := &certification.CertificationResponse{
		Partition: sysID3,
		UC: types.UnicityCertificate{
			Version:                1,
			UnicityTreeCertificate: mockUc.UnicityTreeCertificate,
			UnicitySeal:            mockUc.UnicitySeal,
			InputRecord: &types.InputRecord{Version: 1,
				RoundNumber:  2,
				Hash:         []byte{1, 2, 3},
				PreviousHash: []byte{3, 2, 1},
				SummaryValue: []byte{2, 4, 6, 8},
				BlockHash:    []byte{3, 3, 3},
			}}}
	require.NoError(t, store.Update(3, []*certification.CertificationResponse{newUC}))
	lastCert, err = store.GetCertificate(sysID3)
	require.NoError(t, err)
	require.Equal(t, lastCert, newUC)
	IRmap, err := store.GetLastCertifiedInputRecords()
	require.NoError(t, err)
	require.Contains(t, IRmap, sysID3)
	require.Equal(t, IRmap[sysID3], newUC.UC.InputRecord)
	// read non-existing system id
	lastCert, err = store.GetCertificate(sysID2)
	require.ErrorContains(t, err, "no certificate for partition 00000002 in DB")
	require.Nil(t, lastCert)
	// read sys id 1
	lastCert, err = store.GetCertificate(sysID1)
	require.NoError(t, err)
	require.Equal(t, &lastCert.UC, mockUc)
}

func TestInMemState(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	stateStore := NewStateStore(db)
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
