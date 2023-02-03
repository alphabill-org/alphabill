package store

import (
	gocrypto "crypto"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/stretchr/testify/require"
)

const (
	round             uint64 = 1
	roundCreationTime        = 100000
)

var sysID = p.SystemIdentifier([]byte{0, 0, 0, 0})
var previousHash = make([]byte, gocrypto.SHA256.Size())
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

func TestBoltDB_InvalidPath(t *testing.T) {
	// provide a file that is not a DB file
	store, err := NewBoltStore("testdata/invalid-root-key.json")
	require.Error(t, err)
	require.Nil(t, store)
}

func TestPersistentStore_NewAndUninitiated(t *testing.T) {
	f, err := os.CreateTemp("", "bolt-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	boltDB, err := NewBoltStore(f.Name())
	require.NoError(t, err)
	require.NotNil(t, boltDB)
	state := NewRootState()
	require.ErrorIs(t, boltDB.Read("state", state), ErrValueEmpty)
	require.Empty(t, len(state.Certificates))
	require.Equal(t, uint64(0), state.Round)
	require.Nil(t, state.RootHash)
	// make writing nil fails
	require.Error(t, boltDB.Write("", nil))
	require.Error(t, boltDB.Write("some", nil))
}

func TestNextSave_ok(t *testing.T) {
	f, err := os.CreateTemp("", "bolt-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	boltDB, err := NewBoltStore(f.Name())
	require.NoError(t, err)
	require.NotNil(t, boltDB)
	ucs := map[p.SystemIdentifier]*certificates.UnicityCertificate{sysID: mockUc}
	// initiate state, i.e. boltDB something
	require.NoError(t, boltDB.Write("state", &RootState{Round: round, Certificates: ucs, RootHash: previousHash}))
	state := NewRootState()
	require.NoError(t, boltDB.Read("state", state))
	// check that initial state was saved as intended
	require.Equal(t, round, state.Round)
	require.Equal(t, state.RootHash, previousHash)
	require.Len(t, state.Certificates, 1)
	// update
	uc := &certificates.UnicityCertificate{
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: sysID.Bytes(),
		},
	}
	newHash := []byte{1}
	certs := map[p.SystemIdentifier]*certificates.UnicityCertificate{sysID: uc}
	err = boltDB.Write("state", &RootState{Round: round + 1, Certificates: certs, RootHash: newHash})
	require.NoError(t, err)
	require.NoError(t, boltDB.Read("state", state))
	require.Equal(t, round+1, state.Round)
	require.Equal(t, state.RootHash, newHash)
	require.Len(t, state.Certificates, 1)
	require.Contains(t, state.Certificates, sysID)
	ucStored, found := state.Certificates[sysID]
	require.True(t, found)
	require.Equal(t, uc, ucStored)
}
