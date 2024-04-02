package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUint64MarshalText_OK(t *testing.T) {
	var u Uint64 = 999
	marshaled, err := u.MarshalText()
	require.NoError(t, err)
	require.Equal(t, string(marshaled), "999")

	var unmarshaled Uint64
	err = unmarshaled.UnmarshalText(marshaled)
	require.NoError(t, err)
	require.Equal(t, u, unmarshaled)
}

func TestBytesMarshalText_OK(t *testing.T) {
	var bytes Bytes = []byte{1, 2, 3, 4, 5, 6, 7}
	marshaled, err := bytes.MarshalText()
	require.NoError(t, err)
	require.Equal(t, string(marshaled), "0x01020304050607")

	var unmarshaled Bytes
	err = unmarshaled.UnmarshalText(marshaled)
	require.NoError(t, err)
	require.Equal(t, bytes, unmarshaled)
}

func TestBytesMarshalText_ZeroLengthSliceIsNil(t *testing.T) {
	zeroLengthSlice := make(Bytes, 0)
	marshaled, err := zeroLengthSlice.MarshalText()
	require.NoError(t, err)
	require.Nil(t, marshaled)
}

func TestBytesUnmarshalText_ZeroLengthSliceIsNil(t *testing.T) {
	zeroLengthSlice := make(Bytes, 0)
	var b Bytes
	err := b.UnmarshalText(zeroLengthSlice)
	require.NoError(t, err)
	require.Nil(t, b)
}
