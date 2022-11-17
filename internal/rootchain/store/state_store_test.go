package store

import (
	gocrypto "crypto"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
)

var zeroHash = make([]byte, gocrypto.SHA256.Size())

func TestInMemState_Initialization(t *testing.T) {
	stateStore := NewInMemStateStore(gocrypto.SHA256)
	s, err := stateStore.Get()
	require.NoError(t, err)
	require.Equal(t, s.LatestRound, uint64(0))
	require.Equal(t, s.LatestRootHash, zeroHash)
	require.Equal(t, len(s.Certificates), 0)
}

func TestInMemState_GetAndSave(t *testing.T) {
	stateStore := NewInMemStateStore(gocrypto.SHA256)
	// Save State
	certs := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysId: mockUc}
	require.NoError(t, stateStore.Save(RootState{LatestRound: 1, Certificates: certs, LatestRootHash: []byte{1}}))
	s, err := stateStore.Get()
	require.NoError(t, err)
	require.Equal(t, s.LatestRound, uint64(1))
	// Only thing checked at this point is round number increase
	require.Error(t, stateStore.Save(RootState{LatestRound: 3, Certificates: nil, LatestRootHash: nil}))
}

func TestPersistentRootState_GetAndSave(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	storage := createBoltRootStore(t)
	require.NotNil(t, storage)
	stateStore, err := NewPersistentStateStore(storage)
	require.NoError(t, err)
	s, err := stateStore.Get()
	require.Equal(t, s.LatestRound, uint64(1))
	require.Equal(t, s.LatestRootHash, zeroHash)
	eqCerts := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysId: mockUc}
	require.Equal(t, s.Certificates, eqCerts)
	// Illegal round number
	require.Error(t, stateStore.Save(RootState{LatestRound: 3, Certificates: nil, LatestRootHash: nil}))
}
