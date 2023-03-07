package monolithic

import (
	gocrypto "crypto"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/stretchr/testify/require"
)

const (
	roundCreationTime = 100000
)

var zeroHash = make([]byte, gocrypto.SHA256.Size())
var sysID = protocol.SystemIdentifier([]byte{0, 0, 0, 0})
var mockUc = &certificates.UnicityCertificate{
	UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
		SystemIdentifier:      []byte(sysID),
		SiblingHashes:         nil,
		SystemDescriptionHash: nil,
	},
	UnicitySeal: &certificates.UnicitySeal{
		RootRoundInfo: &certificates.RootRoundInfo{
			RoundNumber:     1,
			Timestamp:       roundCreationTime,
			CurrentRootHash: make([]byte, gocrypto.SHA256.Size()),
		},
		CommitInfo: &certificates.CommitInfo{
			RootRoundInfoHash: make([]byte, gocrypto.SHA256.Size()),
			RootHash:          make([]byte, gocrypto.SHA256.Size()),
		},
	},
}

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
	stateStore, err := NewStateStore(testGenesis, "")
	require.NoError(t, err)
	require.NotNil(t, stateStore)
}

func TestInMemState_GetAndSave(t *testing.T) {
	stateStore, err := NewStateStore(testGenesis, "")
	// Save State
	certs := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysID: mockUc}
	require.NoError(t, stateStore.Save(&RootState{Round: 2, Certificates: certs, RootHash: []byte{1}}))
	lastState := stateStore.Get()
	require.NoError(t, err)
	require.Equal(t, lastState.Round, uint64(2))
	// Root round number can skip rounds, but must not be smaller or equal
	require.Error(t, stateStore.Save(&RootState{Round: 1, Certificates: nil, RootHash: nil}))
	require.Error(t, stateStore.Save(&RootState{Round: 2, Certificates: nil, RootHash: nil}))
	require.NoError(t, stateStore.Save(&RootState{Round: 3, Certificates: nil, RootHash: nil}))
}

func TestPersistentRootState_GetAndSave(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	dir, err := os.MkdirTemp("", "bolt*")
	require.NoError(t, err)
	stateStore, err := NewStateStore(testGenesis, dir)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()
	require.NoError(t, err)
	require.NotNil(t, stateStore)
	s := stateStore.Get()
	require.Equal(t, uint64(1), s.Round)
	require.Equal(t, zeroHash, s.RootHash)
	eqCerts := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{sysID: mockUc}
	require.Equal(t, s.Certificates, eqCerts)
	// Try to store a nil state
	require.Error(t, stateStore.Save(nil))
	// Illegal round number - remove, this check does not belong to a store?
	require.Error(t, stateStore.Save(&RootState{Round: 0, Certificates: nil, RootHash: nil}))
}
