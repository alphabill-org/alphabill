package templates

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestP2pkh256(t *testing.T) {
	sigData := []byte{0x01}
	sig, pubKey := testsig.SignBytes(t, sigData)

	runner := &P2pkh256{}

	payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
	payloadBytes, err := cbor.Marshal(payload)
	require.NoError(t, err)
	signature := &P2pkh256Signature{Sig: sig, PubKey: pubKey}
	sigBytes, err := cbor.Marshal(signature)
	require.NoError(t, err)
	require.NoError(t, runner.Execute(payloadBytes, sigBytes, sigData))
}
