package templates

import (
	"bytes"
	"testing"

	"github.com/alphabill-org/alphabill/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
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
	t.Parallel()

	runner := &P2pkh256{}
	require.Equal(t, P2pkh256ID, runner.ID())

	t.Run("P2pkh256 template ok", func(t *testing.T) {
		sigData := []byte{0x01}
		sig, pubKey := testsig.SignBytes(t, sigData)
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey}
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
		signature := predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, test.RandomBytes(5)), "verification failed")
	})

	t.Run("P2pkh256 template wrong pubkeyhash", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: test.RandomBytes(32)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "pubkey hash does not match")
	})

	t.Run("P2pkh256 template wrong pubkeyhash length", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: test.RandomBytes(34)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "invalid pubkey hash size")
	})

	t.Run("P2pkh256 template wrong signature size", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := predicates.P2pkh256Signature{Sig: sig[:64], PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "invalid signature size")
	})

	t.Run("P2pkh256 template wrong pubkey size", func(t *testing.T) {
		sig, pubKey := testsig.SignBytes(t, []byte{0x01})
		payload := &P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)}
		payloadBytes, err := cbor.Marshal(payload)
		require.NoError(t, err)
		signature := predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey[:32]}
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
		signature := predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey}
		sigBytes, err := cbor.Marshal(signature)
		require.NoError(t, err)
		require.ErrorContains(t, runner.Execute(payloadBytes, sigBytes, nil), "failed to create verifier")
	})
}

func Test_P2pkh256_templateBytes(t *testing.T) {
	t.Parallel()

	/*
		Make sure that CBOR encoder hasn't changed how it encodes our "hardcoded templates"
		or that the constants haven't been changed.
		If these tests fail it's a breaking change!
	*/

	t.Run("always false", func(t *testing.T) {
		buf, err := cbor.Marshal(predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysFalseID}})
		require.NoError(t, err)
		require.True(t, bytes.Equal(buf, alwaysFalseBytes), `CBOR representation of "always false" predicate template has changed (expected %X, got %X)`, alwaysFalseBytes, buf)
	})

	t.Run("always true", func(t *testing.T) {
		buf, err := cbor.Marshal(predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysTrueID}})
		require.NoError(t, err)
		require.True(t, bytes.Equal(buf, alwaysTrueBytes), `CBOR representation of "always true" predicate template has changed (expected %X, got %X)`, alwaysTrueBytes, buf)
	})

	t.Run("p2pkh", func(t *testing.T) {
		require.Equal(t, byte(2), P2pkh256ID, "p2pkh template ID")
	})
}

func Benchmark_P2pkh256Execute(b *testing.B) {
	// random 42 bytes
	payload := []byte{0x16, 0x95, 0xf8, 0xf7, 0xa9, 0xd1, 0x9a, 0xe1, 0xce, 0xf5, 0x45, 0x6, 0xd1, 0x81, 0x2a, 0x1, 0xaa, 0x6d, 0x3e, 0xe1, 0x76, 0x42, 0x2e, 0xfb, 0x3e, 0xae, 0xe2, 0x36, 0xdf, 0x5f, 0xe1, 0x8f, 0x17, 0xa1, 0xf4, 0xad, 0xfa, 0xfa, 0x7c, 0x1e, 0x53, 0x5e}
	// CBOR(P2pkh256Signature{Sig: sign(payload), PubKey: pubKey})
	sigBytes := []byte{0x82, 0x58, 0x41, 0xa8, 0xd8, 0x61, 0xcc, 0x3f, 0x7f, 0x59, 0xf7, 0x7f, 0x8d, 0x65, 0xfd, 0xcc, 0x14, 0xf8, 0x19, 0x80, 0x5e, 0xe2, 0x4b, 0xb8, 0xb9, 0x98, 0x9, 0xad, 0xa, 0x1c, 0x6, 0x2e, 0x90, 0x51, 0xd8, 0x33, 0xe0, 0x9d, 0x47, 0x41, 0x9, 0x72, 0x4c, 0x95, 0xcb, 0x35, 0xcb, 0x33, 0xf, 0x5f, 0xca, 0x2f, 0xe5, 0xb9, 0x9c, 0xf9, 0x8c, 0x7e, 0xb8, 0xb2, 0x34, 0x65, 0xbb, 0x5b, 0x56, 0x5a, 0x36, 0x0, 0x58, 0x21, 0x2, 0x77, 0x84, 0xba, 0xb3, 0x90, 0xc4, 0xf6, 0x86, 0x5b, 0xf7, 0xdb, 0xfc, 0xca, 0xc1, 0x97, 0x4, 0x8f, 0x3d, 0x9e, 0x74, 0x94, 0x55, 0x47, 0x8d, 0x70, 0x66, 0xcb, 0xc7, 0x4d, 0x1b, 0x84, 0x79}
	// CBOR(P2pkh256Payload{PubKeyHash: hash.Sum256(pubKey)})
	payloadBytes := []byte{0x81, 0x58, 0x20, 0xf4, 0xc1, 0x7c, 0x25, 0x9e, 0xec, 0x43, 0xfc, 0xaa, 0xf, 0xa2, 0x2, 0xbe, 0xe5, 0x54, 0xbc, 0xde, 0x8d, 0x44, 0xb0, 0xef, 0x83, 0x1d, 0xaf, 0xb, 0x71, 0x3c, 0x96, 0xe6, 0xd8, 0x47, 0xd9}

	runner := &P2pkh256{}
	// valid data, the P2pkh256.Execute should not return any error
	for i := 0; i < b.N; i++ {
		if err := runner.Execute(payloadBytes, sigBytes, payload); err != nil {
			b.Error(err.Error())
		}
	}
}
