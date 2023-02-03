package store

import (
	gocrypto "crypto"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, gocrypto.SHA256.Size())

var testGenesis = &genesis.RootGenesis{
	Partitions: []*genesis.GenesisPartitionRecord{
		{
			Nodes:       nil,
			Certificate: mockUc,
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: sysID.Bytes(),
				T2Timeout:        2500,
			},
		},
	},
}

func TestInMemState_Initialization(t *testing.T) {
	stateStore := New()
	s, err := stateStore.Get()
	require.ErrorIs(t, ErrValueEmpty, err)
	require.Nil(t, s)
}

func TestInMemState_GetAndSave(t *testing.T) {
	stateStore := New()
	s, err := stateStore.Get()
	require.ErrorIs(t, ErrValueEmpty, err)
	// Save State
	certs := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysID: mockUc}
	require.NoError(t, stateStore.Save(&RootState{Round: 1, Certificates: certs, RootHash: []byte{1}}))
	s, err = stateStore.Get()
	require.NoError(t, err)
	require.Equal(t, s.Round, uint64(1))
	// Root round number can skip rounds, but must not be smaller or equal
	require.Error(t, stateStore.Save(&RootState{Round: 1, Certificates: nil, RootHash: nil}))
	require.NoError(t, stateStore.Save(&RootState{Round: 3, Certificates: nil, RootHash: nil}))
}

func TestPersistentRootState_GetAndSave(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	f, err := os.CreateTemp("", "bolt-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	storage, err := NewBoltStore(f.Name())
	require.NoError(t, err)
	// init storage from DB
	stateStore := New(WithDBStore(storage))
	s, err := stateStore.Get()
	require.ErrorIs(t, ErrValueEmpty, err)
	require.NoError(t, stateStore.Save(NewRootStateFromGenesis(testGenesis)))
	s, err = stateStore.Get()
	require.Equal(t, uint64(1), s.Round)
	require.Equal(t, zeroHash, s.RootHash)
	eqCerts := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysID: mockUc}
	require.Equal(t, s.Certificates, eqCerts)
	// Try to store a nil state
	require.Error(t, stateStore.Save(nil))
	// Illegal round number - remove, this check does not belong to a store?
	require.Error(t, stateStore.Save(&RootState{Round: 0, Certificates: nil, RootHash: nil}))
}
