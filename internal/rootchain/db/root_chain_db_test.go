package db

import (
	gocrypto "crypto"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

const (
	round = 1
)

var sysId = p.SystemIdentifier([]byte{0, 0, 0, 0})
var previousHash = make([]byte, gocrypto.SHA256.Size())
var mockUc = &certificates.UnicityCertificate{
	UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
		SystemIdentifier:      []byte(sysId),
		SiblingHashes:         nil,
		SystemDescriptionHash: nil,
	},
	UnicitySeal: &certificates.UnicitySeal{
		RootChainRoundNumber: 1,
		PreviousHash:         make([]byte, gocrypto.SHA256.Size()),
		Hash:                 make([]byte, gocrypto.SHA256.Size()),
	},
}

func TestPersistentRootStore(t *testing.T) {
	tests := []struct {
		desc  string
		store *RootChainDb
	}{
		{
			desc:  "bolt",
			store: createBoltRootStore(t),
		},
	}

	for _, tt := range tests {
		t.Run("db|"+tt.desc, func(t *testing.T) {
			test_New(t, tt.store)
			test_IR(t, tt.store)
			test_nextRound(t, tt.store)
			test_badNextRound(t, tt.store)
		})
	}
}

func test_New(t *testing.T, rs *RootChainDb) {
	require.NotNil(t, rs)
	require.Equal(t, rs.UCCount(), 1)
	require.Equal(t, rs.ReadLatestRoundNumber(), uint64(1))
}

func test_IR(t *testing.T, rs *RootChainDb) {
	ir := &certificates.InputRecord{
		Hash: []byte{1, 2, 3},
	}

	require.Empty(t, rs.GetAllIRs())

	rs.AddIR(sysId, ir)
	require.Equal(t, ir, rs.GetIR(sysId))
	require.Len(t, rs.GetAllIRs(), 1)
}

func test_nextRound(t *testing.T, rs *RootChainDb) {
	uc := &certificates.UnicityCertificate{
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: sysId.Bytes(),
		},
	}

	round := rs.ReadLatestRoundNumber()
	hash := []byte{1}
	rs.WriteState([]byte{1}, []*certificates.UnicityCertificate{uc}, round+1)

	require.Equal(t, uc, rs.GetUC(sysId))
	require.Equal(t, round+1, rs.ReadLatestRoundNumber())
	require.Equal(t, hash, rs.ReadLatestRoundRootHash())
}

func test_badNextRound(t *testing.T, rs *RootChainDb) {
	uc := &certificates.UnicityCertificate{
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: sysId.Bytes(),
		},
	}

	round := rs.ReadLatestRoundNumber()
	require.PanicsWithError(t, "Inconsistent round number, current=2, new=2", func() {
		rs.WriteState([]byte{1}, []*certificates.UnicityCertificate{uc}, round)
	})
}

func Test_Noinit(t *testing.T) {
	dbFile := path.Join(os.TempDir(), BoltRootChainStoreFileName)
	t.Cleanup(func() {
		err := os.Remove(dbFile)
		if err != nil {
			fmt.Printf("error deleting %s: %v\n", dbFile, err)
		}
	})
	store, err := NewBoltRootChainDb(dbFile)
	require.NoError(t, err)
	require.False(t, store.GetInitiated())
	require.Equal(t, uint64(0), store.ReadLatestRoundNumber())
	require.Nil(t, store.GetUC(sysId))
	require.Nil(t, store.ReadLatestRoundRootHash())
}

func createBoltRootStore(t *testing.T) *RootChainDb {
	dbFile := path.Join(os.TempDir(), BoltRootChainStoreFileName)
	t.Cleanup(func() {
		err := os.Remove(dbFile)
		if err != nil {
			fmt.Printf("error deleting %s: %v\n", dbFile, err)
		}
	})
	store, err := NewBoltRootChainDb(dbFile)
	require.NoError(t, err)
	ucs := []*certificates.UnicityCertificate{mockUc}
	require.NotPanics(t, func() { store.WriteState(previousHash, ucs, round) })
	return store
}
