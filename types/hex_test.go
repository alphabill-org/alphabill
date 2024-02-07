package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBytes_MarshalText(t *testing.T) {
	var bytes Bytes = []byte{1,2,3,4,5,6,7}
	marshaled, err := bytes.MarshalText()
	require.NoError(t, err)
	require.Equal(t, string(marshaled), "0x01020304050607")

	var unmarshaled Bytes
	err = unmarshaled.UnmarshalText(marshaled)
	require.NoError(t, err)
	require.Equal(t, bytes, unmarshaled)
}
