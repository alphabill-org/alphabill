package store

import (
	"fmt"
	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

var sysId = p.SystemIdentifier([]byte{0, 0, 0, 0})

func TestRootStore(t *testing.T) {
	tests := []struct {
		desc  string
		store RootChainStore
	}{
		{
			desc:  "inmemory",
			store: NewInMemoryRootChainStore(),
		},
		{
			desc:  "bolt",
			store: createBoltRootStore(t),
		},
	}

	for _, tt := range tests {
		t.Run("store|"+tt.desc, func(t *testing.T) {
			test_New(t, tt.store)
			test_IR(t, tt.store)
			test_nextRound(t, tt.store)
			test_badNextRound(t, tt.store)
		})
	}
}

func test_New(t *testing.T, rs RootChainStore) {
	require.NotNil(t, rs)
	require.Zero(t, rs.UCCount())
	require.Equal(t, rs.GetRoundNumber(), uint64(1))
}

func test_IR(t *testing.T, rs RootChainStore) {
	ir := &certificates.InputRecord{
		Hash: []byte{1, 2, 3},
	}

	require.Empty(t, rs.GetAllIRs())

	rs.AddIR(sysId, ir)
	require.Equal(t, ir, rs.GetIR(sysId))
	require.Len(t, rs.GetAllIRs(), 1)
}

func test_nextRound(t *testing.T, rs RootChainStore) {
	uc := &certificates.UnicityCertificate{
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: sysId.Bytes(),
		},
	}

	round := rs.GetRoundNumber()
	hash := []byte{1}
	rs.SaveState([]byte{1}, []*certificates.UnicityCertificate{uc}, round+1)

	require.Equal(t, uc, rs.GetUC(sysId))
	require.Equal(t, round+1, rs.GetRoundNumber())
	require.Equal(t, hash, rs.GetPreviousRoundRootHash())
}

func test_badNextRound(t *testing.T, rs RootChainStore) {
	uc := &certificates.UnicityCertificate{
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: sysId.Bytes(),
		},
	}

	round := rs.GetRoundNumber()
	require.PanicsWithError(t, "Inconsistent round number, current=2, new=2", func() {
		rs.SaveState([]byte{1}, []*certificates.UnicityCertificate{uc}, round)
	})
}

func createBoltRootStore(t *testing.T) *BoltRootChainStore {
	dbFile := path.Join(os.TempDir(), BoltRootChainStoreFileName)
	t.Cleanup(func() {
		err := os.Remove(dbFile)
		if err != nil {
			fmt.Printf("error deleting %s: %v\n", dbFile, err)
		}
	})
	store, err := NewBoltRootChainStore(dbFile)
	require.NoError(t, err)
	return store
}
