package predicates

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/stretchr/testify/require"
)

func Test_ExtractPubKey(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		pk, err := ExtractPubKey(nil)
		require.EqualError(t, err, `empty owner proof as input`)
		require.Nil(t, pk)

		pk, err = ExtractPubKey([]byte{})
		require.EqualError(t, err, `empty owner proof as input`)
		require.Nil(t, pk)
	})

	t.Run("invalid CBOR input", func(t *testing.T) {
		pk, err := ExtractPubKey([]byte{0})
		require.EqualError(t, err, `decoding owner proof as Signature: cbor: cannot unmarshal positive integer into Go value of type templates.P2pkh256Signature`)
		require.Nil(t, pk)
	})

	t.Run("success", func(t *testing.T) {
		pubKey := []byte{0x2, 0x12, 0x91, 0x1c, 0x73, 0x41, 0x39, 0x9e, 0x87, 0x68, 0x0, 0xa2, 0x68, 0x85, 0x5c, 0x89, 0x4c, 0x43, 0xeb, 0x84, 0x9a, 0x72, 0xac, 0x5a, 0x9d, 0x26, 0xa0, 0x9, 0x10, 0x41, 0xc1, 0x7, 0xf0}
		ownerProof := templates.NewP2pkh256SignatureBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}, pubKey)
		pk, err := ExtractPubKey(ownerProof)
		require.NoError(t, err)
		require.Equal(t, pubKey, pk)
	})
}
