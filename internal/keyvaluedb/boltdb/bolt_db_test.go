package boltdb

import (
	gocrypto "crypto"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/stretchr/testify/require"
)

const (
	round uint64 = 1
)

var sysID = types.SystemID{0, 0, 0, 1}
var unicityMap = map[types.SystemID32]*types.UnicityCertificate{
	sysID.ToSystemID32(): {
		UnicityTreeCertificate: &types.UnicityTreeCertificate{
			SystemIdentifier:      sysID,
			SiblingHashes:         nil,
			SystemDescriptionHash: nil,
		},
		UnicitySeal: &types.UnicitySeal{
			RootChainRoundNumber: round,
			Hash:                 make([]byte, gocrypto.SHA256.Size()),
		},
	},
}

func isEmpty(t *testing.T, db *BoltDB) bool {
	empty, err := keyvaluedb.IsEmpty(db)
	require.NoError(t, err)
	return empty
}

func initBoltDB(t *testing.T) *BoltDB {
	t.Helper()
	dbDir := t.TempDir()
	boltDB, err := New(filepath.Join(dbDir, "bolt.db"))
	require.NoError(t, err)
	require.NotNil(t, boltDB)
	return boltDB
}

func TestMemDB_TestIsEmpty(t *testing.T) {
	db := initBoltDB(t)
	require.NotNil(t, db)
	empty, err := keyvaluedb.IsEmpty(db)
	require.NoError(t, err)
	require.True(t, empty)
	require.NoError(t, db.Write([]byte("foo"), "test"))
	empty, err = keyvaluedb.IsEmpty(db)
	require.NoError(t, err)
	require.False(t, empty)
	empty, err = keyvaluedb.IsEmpty(nil)
	require.ErrorContains(t, err, "db is nil")
	require.True(t, empty)
}

func TestBoltDB_InvalidPath(t *testing.T) {
	// provide a file that is not a DB file
	store, err := New("testdata/invalid-root-key.json")
	require.Error(t, err)
	require.Nil(t, store)
}

func TestBoltDB_TestEmptyValue(t *testing.T) {
	db := initBoltDB(t)
	var uc types.UnicityCertificate
	found, err := db.Read([]byte("certificate"), &uc)
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, db.Write([]byte("certificate"), &uc))
	var back types.UnicityCertificate
	found, err = db.Read([]byte("certificate"), &back)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, &uc, &back)
}

func TestBoltDB_TestInvalidReadWrite(t *testing.T) {
	db := initBoltDB(t)
	require.NotNil(t, db)
	var uc *types.UnicityCertificate = nil
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
	require.True(t, isEmpty(t, db))
}

func TestBoltDB_TestReadAndWrite(t *testing.T) {
	db := initBoltDB(t)
	require.NotNil(t, db)
	require.True(t, isEmpty(t, db))
	var value uint64 = 1
	require.NoError(t, db.Write([]byte("integer"), value))
	// set empty slice
	require.NoError(t, db.Write([]byte("slice"), []byte{}))
	require.False(t, isEmpty(t, db))
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
	require.NotNil(t, db)
	// use custom type that has got marshal implementation
	c := make(chan int)
	require.Error(t, db.Write([]byte("channel"), c))
}

func TestBoltDB_Delete(t *testing.T) {
	db := initBoltDB(t)
	require.NotNil(t, db)
	require.True(t, isEmpty(t, db))
	var value uint64 = 1
	require.NoError(t, db.Write([]byte("integer"), value))
	require.False(t, isEmpty(t, db))
	require.NoError(t, db.Delete([]byte("integer")))
	require.True(t, isEmpty(t, db))
	// delete non-existing key
	require.NoError(t, db.Delete([]byte("integer")))
	// delete invalid key
	require.Error(t, db.Delete(nil))
}

func TestBoltDB_WriteReadComplexStruct(t *testing.T) {
	db := initBoltDB(t)
	require.NotNil(t, db)
	require.True(t, isEmpty(t, db))
	// write a complex struct
	require.NoError(t, db.Write([]byte("certificates"), unicityMap))
	ucs := make(map[types.SystemID32]*types.UnicityCertificate)
	present, err := db.Read([]byte("certificates"), &ucs)
	require.NoError(t, err)
	require.True(t, present)
	// check that initial state was saved as intended
	require.Len(t, ucs, 1)
	require.Contains(t, ucs, sysID.ToSystemID32())
	uc, _ := ucs[sysID.ToSystemID32()]
	original, _ := unicityMap[sysID.ToSystemID32()]
	require.Equal(t, original, uc)
	// update
	uc.UnicitySeal.Hash = []byte{1}
	newUC := map[types.SystemID32]*types.UnicityCertificate{sysID.ToSystemID32(): uc}
	err = db.Write([]byte("certificates"), newUC)
	require.NoError(t, err)
	present, err = db.Read([]byte("certificates"), &ucs)
	require.NoError(t, err)
	require.True(t, present)
	require.Len(t, ucs, 1)
	require.Contains(t, ucs, sysID.ToSystemID32())
	uc, _ = ucs[sysID.ToSystemID32()]
	require.Equal(t, []byte{1}, uc.UnicitySeal.Hash)
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

func TestBoltDB_TestReadAndWriteIterate(t *testing.T) {
	db := initBoltDB(t)
	require.NotNil(t, db)
	require.True(t, isEmpty(t, db))
	var value uint64 = 1
	require.NoError(t, db.Write([]byte("integer"), value))
	require.NoError(t, db.Write([]byte("test"), "d1"))
	require.NoError(t, db.Write([]byte("test2"), "d2"))
	require.False(t, isEmpty(t, db))
	var back uint64
	found, err := db.Read([]byte("integer"), &back)
	require.NoError(t, err)
	require.True(t, found)
	itr := db.First()
	require.True(t, itr.Valid())
	require.Equal(t, []byte("integer"), itr.Key())
	require.NoError(t, itr.Close())
	require.NoError(t, db.Write([]byte("test3"), "d3"))
	var data string
	found, err = db.Read([]byte("test3"), &data)
	require.NoError(t, err)
	require.True(t, found)
}
