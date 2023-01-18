package store

import (
	gocrypto "crypto"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, gocrypto.SHA256.Size())

func TestInMemState_Initialization(t *testing.T) {
	stateStore := NewInMemStateStore()
	s, err := stateStore.Get()
	require.NoError(t, err)
	require.Equal(t, s.LatestRound, uint64(0))
	require.Nil(t, s.LatestRootHash)
	require.Equal(t, len(s.Certificates), 0)
}

func TestInMemState_GetAndSave(t *testing.T) {
	stateStore := NewInMemStateStore()
	// Save State
	certs := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysID: mockUc}
	require.NoError(t, stateStore.Save(&RootState{LatestRound: 1, Certificates: certs, LatestRootHash: []byte{1}}))
	s, err := stateStore.Get()
	require.NoError(t, err)
	require.Equal(t, s.LatestRound, uint64(1))
	// Root round number can skip rounds, but must not be smaller or equal
	require.Error(t, stateStore.Save(&RootState{LatestRound: 1, Certificates: nil, LatestRootHash: nil}))
	require.NoError(t, stateStore.Save(&RootState{LatestRound: 3, Certificates: nil, LatestRootHash: nil}))

}

func TestPersistentRootState_GetAndSave(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	f, err := os.CreateTemp("", "bolt-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	storage, err := NewBoltStore(f.Name())
	require.NotNil(t, storage)
	// save a state in DB
	ucs := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{"1": mockUc}
	// initiate state, i.e. store something
	require.NoError(t, storage.Write(&RootState{LatestRound: round, Certificates: ucs, LatestRootHash: previousHash}))
	stateStore, err := New(WithDBStore(storage))
	require.NoError(t, err)
	s, err := stateStore.Get()
	require.Equal(t, s.LatestRound, uint64(1))
	require.Equal(t, s.LatestRootHash, zeroHash)
	eqCerts := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysID: mockUc}
	require.Equal(t, s.Certificates, eqCerts)
	// Try to store a nil state
	require.Error(t, stateStore.Save(nil))
	// Illegal round number - remove, this check does not belong to a store?
	require.Error(t, stateStore.Save(&RootState{LatestRound: 0, Certificates: nil, LatestRootHash: nil}))
}
