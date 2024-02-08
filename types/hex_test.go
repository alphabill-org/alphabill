package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalText_OK(t *testing.T) {
	var bytes Bytes = []byte{1, 2, 3, 4, 5, 6, 7}
	marshaled, err := bytes.MarshalText()
	require.NoError(t, err)
	require.Equal(t, string(marshaled), "0x01020304050607")

	var unmarshaled Bytes
	err = unmarshaled.UnmarshalText(marshaled)
	require.NoError(t, err)
	require.Equal(t, bytes, unmarshaled)
}

func TestMarshalText_ZeroLengthSliceIsNil(t *testing.T) {
	zeroLengthSlice := make(Bytes, 0)
	marshaled, err := zeroLengthSlice.MarshalText()
	require.NoError(t, err)
	require.Nil(t, marshaled)
}

func TestUnmarshalText_ZeroLengthSliceIsNil(t *testing.T) {
	zeroLengthSlice := make(Bytes, 0)
	var b Bytes
	err := b.UnmarshalText(zeroLengthSlice)
	require.NoError(t, err)
	require.Nil(t, b)
}
