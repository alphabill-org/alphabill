package util

import (
	"crypto/rand"
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShuffleSliceCopy(t *testing.T) {
	sample := make([]byte, 100)
	_, err := rand.Read(sample)
	if err != nil {
		t.Error("Error generating random bytes:", err)
	}
	result := ShuffleSliceCopy(sample)
	require.ElementsMatch(t, sample, result)
	require.NotEqualValues(t, sample, result)
}

func TestConverter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		targetName string
		byteVal    []byte
		uint64Val  uint64
		uint32Val  uint32
	}{
		{targetName: "Uint64ToBytes", byteVal: []byte{0, 0, 0, 0, 0, 0, 0, 0}, uint64Val: uint64(0)},
		{targetName: "Uint64ToBytes", byteVal: []byte{0, 0, 0, 0, 0, 0, 0, 1}, uint64Val: uint64(1)},
		{targetName: "Uint64ToBytes", byteVal: []byte{255, 255, 255, 255, 255, 255, 255, 255}, uint64Val: math.MaxUint64},
		{targetName: "BytesToUint64", byteVal: []byte{1, 0, 0, 0, 0, 0, 0, 0}, uint64Val: uint64(72057594037927936)},
		{targetName: "BytesToUint64", byteVal: []byte{0, 0, 0, 0, 0, 9, 9, 9}, uint64Val: uint64(592137)},
		{targetName: "BytesToUint64", byteVal: []byte{99, 99, 99, 99, 99, 99, 99, 99}, uint64Val: uint64(7161677110969590627)},
		{targetName: "Uint32ToBytes", byteVal: []byte{0, 0, 0, 0}, uint32Val: uint32(0)},
		{targetName: "Uint32ToBytes", byteVal: []byte{0, 0, 0, 1}, uint32Val: uint32(1)},
		{targetName: "Uint32ToBytes", byteVal: []byte{255, 255, 255, 255}, uint32Val: math.MaxUint32},
		{targetName: "BytesToUint32", byteVal: []byte{1, 0, 0, 0}, uint32Val: uint32(16777216)},
		{targetName: "BytesToUint32", byteVal: []byte{0, 9, 9, 9}, uint32Val: uint32(592137)},
		{targetName: "BytesToUint32", byteVal: []byte{99, 99, 99, 99}, uint32Val: uint32(1667457891)},
	}
	for _, c := range cases {
		switch c.targetName {
		case "Uint64ToBytes":
			got := Uint64ToBytes(c.uint64Val)
			require.Equal(t, c.byteVal, got)
		case "BytesToUint64":
			got := BytesToUint64(c.byteVal)
			require.EqualValues(t, c.uint64Val, got)
		case "Uint32ToBytes":
			got := Uint32ToBytes(c.uint32Val)
			require.Equal(t, c.byteVal, got)
		case "BytesToUint32":
			got := BytesToUint32(c.byteVal)
			require.Equal(t, c.uint32Val, got)
		default:
			t.Error("unexpected test target name")
		}
	}
}

func TestFileExists(t *testing.T) {
	existingFilePath := "testfile.txt"
	nonExistingFilePath := "nonexistentfile.txt"

	_, err := os.Create(existingFilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove(existingFilePath)
	}()

	require.True(t, FileExists(existingFilePath))
	require.False(t, FileExists(nonExistingFilePath))
}

func TestReadAndWriteJsonFile(t *testing.T) {
	tempFilePath := "testfile.json"
	invalidTempFilePath := "invalidtestfile.json"
	defer func() {
		_ = os.Remove(tempFilePath)
		_ = os.Remove(invalidTempFilePath)
	}()

	type TestData struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	testData := TestData{
		Key:   "test_key",
		Value: "test_value",
	}

	err := WriteJsonFile(tempFilePath, &testData)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(invalidTempFilePath, []byte{0, 0, 0, 1}, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Test cases
	tests := []struct {
		name          string
		filePath      string
		expectedData  TestData
		expectedError bool
	}{
		{
			name:          "Valid JSON File",
			filePath:      tempFilePath,
			expectedData:  testData,
			expectedError: false,
		},
		{
			name:          "Non-Existing File",
			filePath:      "nonexistentfile.json",
			expectedData:  TestData{},
			expectedError: true,
		},
		{
			name:          "Invalid JSON File",
			filePath:      invalidTempFilePath,
			expectedData:  TestData{},
			expectedError: true,
		},
	}

	// Run tests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result TestData
			_, err := ReadJsonFile(test.filePath, &result)

			if (err != nil) != test.expectedError {
				t.Errorf("ReadJsonFile(%s) error = %v, expected error: %t", test.filePath, err, test.expectedError)
			}

			if !test.expectedError && result != test.expectedData {
				t.Errorf("ReadJsonFile(%s) result = %v, expected %v", test.filePath, result, test.expectedData)
			}
		})
	}
}

func TestAddUint64(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args    []uint64
		want    uint64
		wantErr bool
	}{
		{nil, 0, false},
		{[]uint64{}, 0, false},
		{[]uint64{0}, 0, false},
		{[]uint64{1}, 1, false},
		{[]uint64{math.MaxUint64}, math.MaxUint64, false},
		{[]uint64{1, 2, 3}, 6, false},
		{[]uint64{1, math.MaxUint64}, 0, true},
		{[]uint64{math.MaxUint64, math.MaxUint64}, math.MaxUint64 - 1, true},
	}
	for _, c := range cases {
		got, overflow, err := AddUint64(c.args...)

		if err != nil && !c.wantErr {
			t.Errorf("AddUint64(%v) got unexpected error: %v", c.args, err)
		}
		if overflow && !c.wantErr {
			t.Errorf("AddUint64(%v) got unexpected overflow: %v", c.args, overflow)
		}
		if err == nil && c.wantErr {
			t.Errorf("AddUint64(%v) expected error but got none", c.args)
		}
		if got != c.want {
			t.Errorf("AddUint64(%v) got %d, want %d", c.args, got, c.want)
		}
	}
}

func TestStack(t *testing.T) {
	var items Stack[*int]
	require.True(t, items.IsEmpty())
	require.Panics(t, func() { items.Pop() })

	items.Push(nil)
	require.False(t, items.IsEmpty())

	var myInt int = 123
	items.Push(&myInt)
	require.Equal(t, len(items), 2)
	require.Equal(t, *items.Pop(), 123)
	require.Equal(t, items.Pop(), (*int)(nil))
}

func TestIsValidURI(t *testing.T) {
	require.True(t, IsValidURI("https://alphabill.org"))
	require.True(t, IsValidURI("ldap://[2001:db8::7]/c=GB?objectClass?one"))
	require.True(t, IsValidURI("ftp://ftp.is.co.za/rfc/rfc1808.txt"))
	require.True(t, IsValidURI("mailto:John.Doe@example.com"))
	require.True(t, IsValidURI("telnet://192.0.2.16:80/"))
	require.True(t, IsValidURI("urn:oasis:names:specification:docbook:dtd:xml:4.1.2"))
	require.False(t, IsValidURI("invalid"))
}
