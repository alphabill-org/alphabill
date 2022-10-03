package store

import (
	gocrypto "crypto"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/stretchr/testify/require"
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
		store *BoltStore
	}{
		{
			desc:  "bolt",
			store: createBoltRootStore(t),
		},
	}

	for _, tt := range tests {
		t.Run("db|"+tt.desc, func(t *testing.T) {
			testNew(t, tt.store)
			testIR(t, tt.store)
			testNextRound(t, tt.store)
			testBadNextRound(t, tt.store)
		})
	}
}

func testNew(t *testing.T, rs *BoltStore) {
	require.NotNil(t, rs)
	cnt, err := rs.CountUC()
	require.Equal(t, cnt, 1)
	require.NoError(t, err)
	round, err := rs.ReadLatestRoundNumber()
	require.NoError(t, err)
	require.Equal(t, round, uint64(1))
}

func testIR(t *testing.T, rs *BoltStore) {
	ir := &certificates.InputRecord{
		Hash: []byte{1, 2, 3},
	}
	all, err := rs.GetAllIRs()
	require.NoError(t, err)
	require.Empty(t, all)

	require.NoError(t, rs.AddIR(sysId, ir))
	ir1, err := rs.GetIR(sysId)
	require.NoError(t, err)
	require.Equal(t, ir, ir1)
	all, err = rs.GetAllIRs()
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func testNextRound(t *testing.T, rs *BoltStore) {
	uc := &certificates.UnicityCertificate{
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: sysId.Bytes(),
		},
	}

	round, err := rs.ReadLatestRoundNumber()
	require.NoError(t, err)
	hash := []byte{1}
	certs := map[p.SystemIdentifier]*certificates.UnicityCertificate{"1": uc}
	err = rs.WriteState(RootState{LatestRound: round + 1, Certificates: certs, LatestRootHash: []byte{1}})
	require.NoError(t, err)
	uc1, err := rs.GetUC(sysId)
	require.NoError(t, err)
	require.Equal(t, uc, uc1)
	r, err := rs.ReadLatestRoundNumber()
	require.Equal(t, round+1, r)
	require.NoError(t, err)
	h, err := rs.ReadLatestRoundRootHash()
	require.NoError(t, err)
	require.Equal(t, hash, h)
}

func testBadNextRound(t *testing.T, rs *BoltStore) {
	uc := &certificates.UnicityCertificate{
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: sysId.Bytes(),
		},
	}
	round, err := rs.ReadLatestRoundNumber()
	require.NoError(t, err)
	certs := map[p.SystemIdentifier]*certificates.UnicityCertificate{"1": uc}
	err = rs.WriteState(RootState{LatestRound: round, Certificates: certs, LatestRootHash: []byte{1}})
	require.ErrorContains(t, err, "Inconsistent round number, current=2, new=2")
}

func createBoltRootStore(t *testing.T) *BoltStore {
	dbFile := path.Join(os.TempDir(), BoltRootChainStoreFileName)
	t.Cleanup(func() {
		err := os.Remove(dbFile)
		if err != nil {
			fmt.Printf("error deleting %s: %v\n", dbFile, err)
		}
	})
	store, err := NewBoltStore(dbFile)
	require.NoError(t, err)
	ucs := map[p.SystemIdentifier]*certificates.UnicityCertificate{"1": mockUc}
	require.NoError(t, store.WriteState(RootState{LatestRound: round, Certificates: ucs, LatestRootHash: previousHash}))
	return store
}
