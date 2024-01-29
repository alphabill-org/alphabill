package templates

import (
	"testing"

	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestAlwaysTrue(t *testing.T) {
	t.Parallel()

	runner := &AlwaysTrue{}
	require.EqualValues(t, AlwaysTrueID, runner.ID())

	t.Run("nil signature", func(t *testing.T) {
		require.NoError(t, runner.Execute(nil, nil, nil))
	})

	t.Run("empty signature", func(t *testing.T) {
		require.NoError(t, runner.Execute(nil, []byte{}, nil))
	})

	t.Run("non-empty signature", func(t *testing.T) {
		require.ErrorContains(t, runner.Execute(nil, []byte{0x01}, nil), "always true predicate requires signature to be empty")
	})

	t.Run("non-empty signature data", func(t *testing.T) {
		require.NoError(t, runner.Execute(nil, nil, []byte{0x01}))
	})

	t.Run("cbor null as signature", func(t *testing.T) {
		require.NoError(t, runner.Execute(nil, cborNull, nil))
	})
}

func TestAlwaysFalse(t *testing.T) {
	t.Parallel()

	runner := &AlwaysFalse{}
	require.EqualValues(t, AlwaysFalseID, runner.ID())

	t.Run("nil signature", func(t *testing.T) {
		require.Error(t, runner.Execute(nil, nil, nil))
	})

	t.Run("empty signature", func(t *testing.T) {
		require.Error(t, runner.Execute(nil, []byte{}, nil))
	})

	t.Run("non-empty signature", func(t *testing.T) {
		require.Error(t, runner.Execute(nil, []byte{0x01}, nil))
	})

	t.Run("non-empty signature data", func(t *testing.T) {
		require.Error(t, runner.Execute(nil, nil, []byte{0x01}))
	})

	t.Run("cbor null as signature", func(t *testing.T) {
		require.Error(t, runner.Execute(nil, cborNull, nil))
	})
}

func TestP2pkh256_Execute(t *testing.T) {
	runner := &P2pkh256{}
	require.Equal(t, P2pkh256ID, runner.ID())

	t.Run("P2pkh256 template ok", func(t *testing.T) {
		sigData := []byte{0x01}
		sig, pubKey := testsig.SignBytes(t, sigData)
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := &P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.NoError(t, runner.Execute(payloadBytes, sigBytes, sigData))
	})

	t.Run("P2pkh256 template decoding failure", func(t *testing.T) {
		require.ErrorContains(t, runner.Execute(test.RandomBytes(10), nil, test.RandomBytes(5)), "failed to decode P2PKH256 predicate template")
	})

	t.Run("P2pkh256 template wrong signature", func(t *testing.T) {
		_, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, test.RandomBytes(10), nil), "failed to decode P2PKH256 signature")
	})

	t.Run("P2pkh256 template wrong signature data", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := &P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, test.RandomBytes(5)), "verification failed")
	})

	t.Run("P2pkh256 template wrong pubkeyhash", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: test.RandomBytes(32)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := &P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "pubkey hash does not match")
	})

	t.Run("P2pkh256 template wrong pubkeyhash length", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: test.RandomBytes(34)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := &P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "invalid pubkey hash size")
	})

	t.Run("P2pkh256 template wrong signature size", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := &P2pkh256Signature{Sig: sig[:64], PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "invalid signature size")
	})

	t.Run("P2pkh256 template wrong pubkey size", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := &P2pkh256Signature{Sig: sig, PubKey: pubKey[:32]}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "invalid pubkey size")
	})

	t.Run("P2pkh256 bad pubkey", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		// set incorrect first byte, compressed key should start with 0x02 or 0x03
		pubKey[0] = 0x00
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := &P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "failed to create verifier")
	})
}
