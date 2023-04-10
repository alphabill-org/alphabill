package memorydb

import (
	gocrypto "crypto"
	"encoding/json"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/stretchr/testify/require"
)

const (
	round uint64 = 1
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
			RootChainRoundNumber: round,
			Hash:                 make([]byte, gocrypto.SHA256.Size()),
		},
	},
}

func isEmpty(t *testing.T, db *MemoryDB) bool {
	empty, err := keyvaluedb.IsEmpty(db)
	require.NoError(t, err)
	return empty
}

func TestMemDB_TestIsEmpty(t *testing.T) {
	db := New()
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

func TestMemDB_TestEmptyValue(t *testing.T) {
	db := New()
	require.NotNil(t, db)
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

func TestMemDB_TestInvalidWriteAndRead(t *testing.T) {
	db := New()
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
	require.True(t, isEmpty(t, db))
}

func TestMemDB_WriteAndRead(t *testing.T) {
	db := New()
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

func TestMemDB_TestSerializeError(t *testing.T) {
	db := New()
	require.NotNil(t, db)
	// use custom type that has got marshal implementation
	c := make(chan int)
	require.Error(t, db.Write([]byte("channel"), c))
}

func TestMemDB_Delete(t *testing.T) {
	db := New()
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

func TestMemDB_WriteReadComplexStruct(t *testing.T) {
	db := New()
	require.NotNil(t, db)
	require.True(t, isEmpty(t, db))
	require.NoError(t, db.Write([]byte("certificates"), unicityMap))
	ucs := make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	present, err := db.Read([]byte("certificates"), &ucs)
	require.NoError(t, err)
	require.True(t, present)
	require.Len(t, ucs, 1)
	require.Contains(t, ucs, sysID)
	uc, _ := ucs[sysID]
	original, _ := unicityMap[sysID]
	require.Equal(t, original, uc)
	// update
	uc.UnicitySeal.Hash = []byte{1}
	newUC := map[p.SystemIdentifier]*certificates.UnicityCertificate{sysID: uc}
	err = db.Write([]byte("certificates"), newUC)
	require.NoError(t, err)
	present, err = db.Read([]byte("certificates"), &ucs)
	require.NoError(t, err)
	require.True(t, present)
	require.Len(t, ucs, 1)
	require.Contains(t, ucs, sysID)
	uc, _ = ucs[sysID]
	require.Equal(t, []byte{1}, uc.UnicitySeal.Hash)
}

func TestMemDB_StartTxNil(t *testing.T) {
	db := &MemoryDB{
		db:      nil,
		encoder: json.Marshal,
		decoder: json.Unmarshal,
	}
	tx, err := db.StartTx()
	require.Error(t, err)
	require.Nil(t, tx)
}
