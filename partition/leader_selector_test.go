package partition

import (
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill/network"
)

func Test_Leader(t *testing.T) {
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	nodeID, err := network.NodeIDFromPublicKeyBytes(pubKey)
	require.NoError(t, err)

	ls := &Leader{}
	require.Equal(t, UnknownLeader, ls.Get().String())
	require.False(t, ls.IsLeader(nodeID))

	require.EqualError(t, ls.Set("foobar"), `decoding node ID "foobar": failed to parse peer ID: invalid cid: encoding/hex: invalid byte: U+006F 'o'`)
	require.Equal(t, UnknownLeader, ls.Get().String())
	require.False(t, ls.IsLeader(nodeID))

	require.NoError(t, ls.Set(nodeID.String()))
	require.Equal(t, nodeID, ls.Get())
	require.True(t, ls.IsLeader(nodeID))
}
