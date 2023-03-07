package boltdb

import (
	gocrypto "crypto"
	"encoding/json"
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

var sysID = p.SystemIdentifier([]byte{0, 0, 0, 1})
var unicityMap = map[p.SystemIdentifier]*certificates.UnicityCertificate{
	sysID: {
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier:      []byte(sysID),
			SiblingHashes:         nil,
			SystemDescriptionHash: nil,
		},
		UnicitySeal: &certificates.UnicitySeal{
			RootRoundInfo: &certificates.RootRoundInfo{
				RoundNumber:     round,
				Timestamp:       roundCreationTime,
				CurrentRootHash: make([]byte, gocrypto.SHA256.Size()),
			},
			CommitInfo: &certificates.CommitInfo{
				RootRoundInfoHash: make([]byte, gocrypto.SHA256.Size()),
				RootHash:          make([]byte, gocrypto.SHA256.Size()),
			},
		},
	},
}

func initBoltDB(t *testing.T) *BoltDB {
	t.Helper()
	f, err := os.CreateTemp("", "bolt-*.db")
	require.NoError(t, err)
	boltDB, err := New(f.Name())
	require.NoError(t, err)
	require.NotNil(t, boltDB)
	return boltDB
}

func TestBoltDB_InvalidPath(t *testing.T) {
	// provide a file that is not a DB file
	store, err := New("testdata/invalid-root-key.json")
	require.Error(t, err)
	require.Nil(t, store)
}

func TestBoltDB_TestEmptyValue(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	var uc certificates.UnicityCertificate
	found, err := db.Read([]byte("certificate"), &uc)
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, db.Write([]byte("certificate"), &uc))
	var back certificates.UnicityCertificate
	found, err = db.Read([]byte("certificate"), &back)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, &uc, &back)
}

func TestBoltDB_TestInvalidReadWrite(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.NotNil(t, db)
	var uc *certificates.UnicityCertificate = nil
	require.Error(t, db.Write([]byte("certificate"), uc))
	require.Error(t, db.Write([]byte(""), nil))
	var value uint64 = 1
	require.Error(t, db.Write(nil, value))
	found, err := db.Read(nil, uc)
	require.Error(t, err)
	require.False(t, found)
	found, err = db.Read(nil, &value)
	require.Error(t, err)
	require.False(t, found)
	found, err = db.Read([]byte{}, &value)
	require.Error(t, err)
	require.False(t, found)
	// check key presents
	found, err = db.Read([]byte("test"), nil)
	require.NoError(t, err)
	require.False(t, found)
	require.True(t, db.Empty())
}

func TestBoltDB_TestReadAndWrite(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.NotNil(t, db)
	require.True(t, db.Empty())
	var value uint64 = 1
	require.NoError(t, db.Write([]byte("integer"), value))
	// set empty slice
	require.NoError(t, db.Write([]byte("slice"), []byte{}))
	require.False(t, db.Empty())
	var some []byte
	found, err := db.Read([]byte("slice"), &some)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, some)
	var back uint64
	found, err = db.Read([]byte("integer"), &back)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, uint64(1), back)
	// wrong type slice
	found, err = db.Read([]byte("slice"), &back)
	require.ErrorContains(t, err, "json: cannot unmarshal string into Go value of type uint64")
	require.True(t, found)
}

func TestBoltDB_TestSerializeError(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.NotNil(t, db)
	// use custom type that has got marshal implementation
	c := make(chan int)
	require.Error(t, db.Write([]byte("channel"), c))
}

func TestBoltDB_Delete(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.NotNil(t, db)
	require.True(t, db.Empty())
	var value uint64 = 1
	require.NoError(t, db.Write([]byte("integer"), value))
	require.False(t, db.Empty())
	require.NoError(t, db.Delete([]byte("integer")))
	require.True(t, db.Empty())
	// delete non-existing key
	require.NoError(t, db.Delete([]byte("integer")))
	// delete invalid key
	require.Error(t, db.Delete(nil))
}

func TestBoltDB_WriteReadComplexStruct(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	// write a complex struct
	require.NoError(t, db.Write([]byte("certificates"), unicityMap))
	ucs := make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	present, err := db.Read([]byte("certificates"), &ucs)
	require.NoError(t, err)
	require.True(t, present)
	// check that initial state was saved as intended
	require.Len(t, ucs, 1)
	require.Contains(t, ucs, sysID)
	uc, _ := ucs[sysID]
	original, _ := unicityMap[sysID]
	require.Equal(t, original, uc)
	// update
	uc.UnicitySeal.RootRoundInfo.CurrentRootHash = []byte{1}
	newUC := map[p.SystemIdentifier]*certificates.UnicityCertificate{sysID: uc}
	err = db.Write([]byte("certificates"), newUC)
	require.NoError(t, err)
	present, err = db.Read([]byte("certificates"), &ucs)
	require.NoError(t, err)
	require.True(t, present)
	require.Len(t, ucs, 1)
	require.Contains(t, ucs, sysID)
	uc, _ = ucs[sysID]
	require.Equal(t, []byte{1}, uc.UnicitySeal.RootRoundInfo.CurrentRootHash)
}

func TestBoltDB_StartTxNil(t *testing.T) {
	db := &BoltDB{
		db:      nil,
		bucket:  []byte("test"),
		encoder: json.Marshal,
		decoder: json.Unmarshal,
	}
	tx, err := db.StartTx()
	require.Error(t, err)
	require.Nil(t, tx)
}
