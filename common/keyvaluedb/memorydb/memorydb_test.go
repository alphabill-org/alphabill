package memorydb

import (
	"encoding/json"
	"testing"

	"github.com/alphabill-org/alphabill/common/keyvaluedb"
	"github.com/stretchr/testify/require"
)

type testStruct struct {
	Name   string
	Data   []byte
	Params struct{ Value []byte }
}

var testDataStruct = map[uint32]*testStruct{
	1: {
		Name: "test",
		Data: []byte{1, 2, 3},
		Params: struct {
			Value []byte
		}{
			Value: []byte{9, 1, 1},
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
	var data testStruct
	found, err := db.Read([]byte("data"), &data)
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, db.Write([]byte("data"), &testDataStruct))
	var back map[uint32]*testStruct
	found, err = db.Read([]byte("data"), &back)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, testDataStruct, back)
}

func TestMemDB_TestInvalidWriteAndRead(t *testing.T) {
	db := New()
	require.NotNil(t, db)
	var data *testStruct = nil
	require.Error(t, db.Write([]byte("data"), data))
	require.Error(t, db.Write([]byte(""), nil))
	var value uint64 = 1
	require.Error(t, db.Write(nil, value))
	found, err := db.Read(nil, data)
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
	// write a complex struct
	require.NoError(t, db.Write([]byte("data"), testDataStruct))
	readData := make(map[uint32]*testStruct)
	present, err := db.Read([]byte("data"), &readData)
	require.NoError(t, err)
	require.True(t, present)
	// check that initial state was saved as intended
	require.Len(t, testDataStruct, 1)
	require.Contains(t, readData, uint32(1))
	value := readData[1]
	original := testDataStruct[1]
	require.Equal(t, original, value)
	// update
	value.Data = []byte{1}
	newData := map[uint32]*testStruct{1: value}
	err = db.Write([]byte("data"), newData)
	require.NoError(t, err)
	present, err = db.Read([]byte("data"), &readData)
	require.NoError(t, err)
	require.True(t, present)
	require.Len(t, readData, 1)
	require.Contains(t, readData, uint32(1))
	value = readData[1]
	require.Equal(t, []byte{1}, value.Data)
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

func TestBoltDB_TestReadAndWriteIterate(t *testing.T) {
	db := New()
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
