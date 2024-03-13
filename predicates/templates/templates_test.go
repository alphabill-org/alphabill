package templates

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestAlwaysTrue(t *testing.T) {
	t.Parallel()

	t.Run("valid arguments", func(t *testing.T) {
		// nil, empty slice and CBOR null are valid arguments for
		// "always true", in any combination
		var args = []struct {
			params []byte
			args   []byte
		}{
			{params: nil, args: nil},
			{params: nil, args: []byte{}},
			{params: []byte{}, args: nil},
			{params: []byte{}, args: []byte{}},
			{params: nil, args: cborNull},
			{params: cborNull, args: nil},
			{params: cborNull, args: cborNull},
			{params: cborNull, args: []byte{}},
			{params: []byte{}, args: cborNull},
		}

		for _, tc := range args {
			res, err := alwaysTrue_Execute(tc.params, tc.args)
			if err != nil {
				t.Errorf("unexpected error with arguments (%#v , %#v): %v", tc.params, tc.args, err)
			}
			if !res {
				t.Errorf("unexpectedly got 'false' for (%#v , %#v)", tc.params, tc.args)
			}
		}
	})

	t.Run("invalid arguments", func(t *testing.T) {
		var args = []struct {
			params []byte
			args   []byte
		}{
			{params: nil, args: []byte{1}},
			{params: []byte{1}, args: nil},
			{params: []byte{0}, args: []byte{0}},
			{params: nil, args: []byte{0xf6, 0}}, // CBOR null with extra byte!
		}

		for _, tc := range args {
			res, err := alwaysTrue_Execute(tc.params, tc.args)
			if err == nil {
				t.Errorf("expected error with arguments (%#v , %#v)", tc.params, tc.args)
			} else if err.Error() != `"always true" predicate arguments must be empty` {
				t.Errorf("unexpected error with arguments (%#v , %#v): %v", tc.params, tc.args, err)
			}
			if res {
				t.Errorf("unexpectedly got 'true' for (%#v , %#v)", tc.params, tc.args)
			}
		}
	})
}

func TestAlwaysFalse(t *testing.T) {
	t.Parallel()

	t.Run("valid arguments", func(t *testing.T) {
		// nil, empty slice and CBOR null are valid arguments for
		// "always false", in any combination
		var args = []struct {
			params []byte
			args   []byte
		}{
			{params: nil, args: nil},
			{params: nil, args: []byte{}},
			{params: []byte{}, args: nil},
			{params: []byte{}, args: []byte{}},
			{params: nil, args: cborNull},
			{params: cborNull, args: nil},
			{params: cborNull, args: cborNull},
			{params: cborNull, args: []byte{}},
			{params: []byte{}, args: cborNull},
		}

		for _, tc := range args {
			res, err := alwaysFalse_Execute(tc.params, tc.args)
			if err != nil {
				t.Errorf("unexpected error with arguments (%#v , %#v): %v", tc.params, tc.args, err)
			}
			if res {
				t.Errorf("unexpectedly got 'true' for (%#v , %#v)", tc.params, tc.args)
			}
		}
	})

	t.Run("invalid arguments", func(t *testing.T) {
		var args = []struct {
			params []byte
			args   []byte
		}{
			{params: nil, args: []byte{1}},
			{params: []byte{1}, args: nil},
			{params: []byte{0}, args: []byte{0}},
			{params: nil, args: []byte{0xf6, 0}}, // CBOR null with extra byte!
		}

		for _, tc := range args {
			res, err := alwaysFalse_Execute(tc.params, tc.args)
			if err == nil {
				t.Errorf("expected error with arguments (%#v , %#v)", tc.params, tc.args)
			} else if err.Error() != `"always false" predicate arguments must be empty` {
				t.Errorf("unexpected error with arguments (%#v , %#v): %v", tc.params, tc.args, err)
			}
			if res {
				t.Errorf("unexpectedly got 'true' for (%#v , %#v)", tc.params, tc.args)
			}
		}
	})
}

func TestP2pkh256_Execute(t *testing.T) {
	t.Parallel()

	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pubKeyHash := hash.Sum256(pubKey)

	validTxOrder := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID: 1,
			Type:     "made up",
			UnitID:   []byte{0, 0, 1, 1, 2, 2},
		},
	}
	require.NoError(t, validTxOrder.Payload.SetAttributes("not really attributes"))
	require.NoError(t, validTxOrder.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))

	execEnv := &mockTxContext{
		payloadBytes: func(txo *types.TransactionOrder) ([]byte, error) { return txo.PayloadBytes() },
	}

	t.Run("success", func(t *testing.T) {
		res, err := p2pkh256_Execute(pubKeyHash, validTxOrder.OwnerProof, validTxOrder, execEnv)
		require.NoError(t, err)
		require.True(t, res)
	})

	t.Run("failure to read txo payload bytes", func(t *testing.T) {
		expErr := errors.New("nope")
		execEnv := &mockTxContext{
			payloadBytes: func(txo *types.TransactionOrder) ([]byte, error) { return nil, expErr },
		}
		res, err := p2pkh256_Execute(pubKeyHash, validTxOrder.OwnerProof, validTxOrder, execEnv)
		require.ErrorIs(t, err, expErr)
		require.False(t, res)
	})

	t.Run("invalid CBOR encoded OwnerProof", func(t *testing.T) {
		res, err := p2pkh256_Execute(pubKeyHash, nil, validTxOrder, execEnv)
		require.EqualError(t, err, `failed to decode P2PKH256 signature: EOF`)
		require.False(t, res)

		res, err = p2pkh256_Execute(pubKeyHash, []byte{}, validTxOrder, execEnv)
		require.EqualError(t, err, `failed to decode P2PKH256 signature: EOF`)
		require.False(t, res)

		res, err = p2pkh256_Execute(pubKeyHash, []byte{0, 1, 2}, validTxOrder, execEnv)
		require.EqualError(t, err, `failed to decode P2PKH256 signature: cbor: 2 bytes of extraneous data starting at index 1`)
		require.False(t, res)
	})

	t.Run("invalid OwnerProof data", func(t *testing.T) {
		// valid owner proof struct (CBOR decoding is success) but invald data inside
		// signature is of invalid length
		signature := predicates.P2pkh256Signature{Sig: []byte{1, 2, 3}, PubKey: pubKey}
		ownerProof, err := cbor.Marshal(signature)
		require.NoError(t, err)
		res, err := p2pkh256_Execute(pubKeyHash, ownerProof, validTxOrder, execEnv)
		require.EqualError(t, err, `invalid signature size: expected 65, got 3 (010203)`)
		require.False(t, res)

		// pubkey is of invalid length
		signature = predicates.P2pkh256Signature{Sig: make([]byte, 65), PubKey: []byte{4, 5, 6}}
		ownerProof, err = cbor.Marshal(signature)
		require.NoError(t, err)
		res, err = p2pkh256_Execute(pubKeyHash, ownerProof, validTxOrder, execEnv)
		require.EqualError(t, err, `invalid pubkey size: expected 33, got 3 (040506)`)
		require.False(t, res)

		// pubKey hash doesn't match with the has of the pubKey in OP
		signature = predicates.P2pkh256Signature{Sig: make([]byte, 65), PubKey: make([]byte, 33)}
		ownerProof, err = cbor.Marshal(signature)
		require.NoError(t, err)
		res, err = p2pkh256_Execute(pubKeyHash, ownerProof, validTxOrder, execEnv)
		require.EqualError(t, err, `pubkey hash does not match`)
		require.False(t, res)

		// set incorrect first byte, compressed key should start with 0x02 or 0x03
		signature = predicates.P2pkh256Signature{Sig: make([]byte, 65), PubKey: make([]byte, 33)}
		ownerProof, err = cbor.Marshal(signature)
		require.NoError(t, err)
		res, err = p2pkh256_Execute(hash.Sum256(signature.PubKey), ownerProof, validTxOrder, execEnv)
		require.EqualError(t, err, `failed to create verifier: public key decompress faield`)
		require.False(t, res)
	})

	t.Run("OwnerProof doesn't verify", func(t *testing.T) {
		// create valid owner proof struct but invald data inside
		// signature is of correct length but invalid
		signature := predicates.P2pkh256Signature{Sig: make([]byte, 65), PubKey: pubKey}
		ownerProof, err := cbor.Marshal(signature)
		require.NoError(t, err)
		res, err := p2pkh256_Execute(pubKeyHash, ownerProof, validTxOrder, execEnv)
		require.EqualError(t, err, `failed to verify signature: verification failed`)
		require.False(t, res)
	})

	t.Run("invalid pubkey hash size", func(t *testing.T) {
		res, err := p2pkh256_Execute(pubKeyHash[:len(pubKeyHash)-1], validTxOrder.OwnerProof, validTxOrder, execEnv)
		require.ErrorContains(t, err, `invalid pubkey hash size: expected 32, got 31`)
		require.False(t, res)
	})
}

func Test_templateBytes(t *testing.T) {
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
		require.True(t, bytes.Equal(alwaysFalseBytes, AlwaysFalseBytes()))
		pred := &predicates.Predicate{}
		require.NoError(t, cbor.Unmarshal(buf, pred))
		require.Equal(t, pred.Code[0], AlwaysFalseID, "always false predicate ID")
	})

	t.Run("always true", func(t *testing.T) {
		buf, err := cbor.Marshal(predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysTrueID}})
		require.NoError(t, err)
		require.True(t, bytes.Equal(buf, alwaysTrueBytes), `CBOR representation of "always true" predicate template has changed (expected %X, got %X)`, alwaysTrueBytes, buf)
		require.True(t, bytes.Equal(alwaysTrueBytes, AlwaysTrueBytes()))
		pred := &predicates.Predicate{}
		require.NoError(t, cbor.Unmarshal(buf, pred))
		require.Equal(t, pred.Code[0], AlwaysTrueID, "always true predicate ID")
	})

	t.Run("p2pkh", func(t *testing.T) {
		pubKeyHash, err := hex.DecodeString("F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0")
		require.NoError(t, err)
		buf, err := cbor.Marshal(predicates.Predicate{Tag: TemplateStartByte, Code: []byte{P2pkh256ID}, Params: pubKeyHash})
		require.NoError(t, err)

		fromHex, err := hex.DecodeString("830041025820F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0")
		require.NoError(t, err)

		require.Equal(t, buf, fromHex)
	})
}

func Test_ExtractPubKeyHashFromP2pkhPredicate(t *testing.T) {
	pubKeyHash, err := hex.DecodeString("F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0")
	require.NoError(t, err)
	result, err := ExtractPubKeyHashFromP2pkhPredicate(NewP2pkh256BytesFromKeyHash(pubKeyHash))
	require.NoError(t, err)
	require.Equal(t, pubKeyHash, result)
}

func Test_IsP2pkhTemplate(t *testing.T) {
	t.Parallel()

	t.Run("p2pkh template true", func(t *testing.T) {
		require.True(t, IsP2pkhTemplate(&predicates.Predicate{Tag: TemplateStartByte, Code: []byte{P2pkh256ID}}))
	})

	t.Run("p2pkh template false", func(t *testing.T) {
		require.False(t, IsP2pkhTemplate(nil))
		require.False(t, IsP2pkhTemplate(&predicates.Predicate{}))
		require.False(t, IsP2pkhTemplate(&predicates.Predicate{Tag: 999, Code: []byte{P2pkh256ID}}))
		require.False(t, IsP2pkhTemplate(&predicates.Predicate{Tag: TemplateStartByte, Code: []byte{P2pkh256ID, P2pkh256ID}}))
	})
}

func Benchmark_P2pkh256Execute(b *testing.B) {
	// random 42 bytes
	payload := []byte{0x16, 0x95, 0xf8, 0xf7, 0xa9, 0xd1, 0x9a, 0xe1, 0xce, 0xf5, 0x45, 0x6, 0xd1, 0x81, 0x2a, 0x1, 0xaa, 0x6d, 0x3e, 0xe1, 0x76, 0x42, 0x2e, 0xfb, 0x3e, 0xae, 0xe2, 0x36, 0xdf, 0x5f, 0xe1, 0x8f, 0x17, 0xa1, 0xf4, 0xad, 0xfa, 0xfa, 0x7c, 0x1e, 0x53, 0x5e}
	// CBOR(P2pkh256Signature{Sig: sign(payload), PubKey: pubKey})
	ownerProof := []byte{0x82, 0x58, 0x41, 0xa8, 0xd8, 0x61, 0xcc, 0x3f, 0x7f, 0x59, 0xf7, 0x7f, 0x8d, 0x65, 0xfd, 0xcc, 0x14, 0xf8, 0x19, 0x80, 0x5e, 0xe2, 0x4b, 0xb8, 0xb9, 0x98, 0x9, 0xad, 0xa, 0x1c, 0x6, 0x2e, 0x90, 0x51, 0xd8, 0x33, 0xe0, 0x9d, 0x47, 0x41, 0x9, 0x72, 0x4c, 0x95, 0xcb, 0x35, 0xcb, 0x33, 0xf, 0x5f, 0xca, 0x2f, 0xe5, 0xb9, 0x9c, 0xf9, 0x8c, 0x7e, 0xb8, 0xb2, 0x34, 0x65, 0xbb, 0x5b, 0x56, 0x5a, 0x36, 0x0, 0x58, 0x21, 0x2, 0x77, 0x84, 0xba, 0xb3, 0x90, 0xc4, 0xf6, 0x86, 0x5b, 0xf7, 0xdb, 0xfc, 0xca, 0xc1, 0x97, 0x4, 0x8f, 0x3d, 0x9e, 0x74, 0x94, 0x55, 0x47, 0x8d, 0x70, 0x66, 0xcb, 0xc7, 0x4d, 0x1b, 0x84, 0x79}

	pk, err := predicates.ExtractPubKey(ownerProof)
	if err != nil {
		b.Error(err.Error())
	}
	pubKeyHash := hash.Sum256(pk)

	execEnv := &mockTxContext{
		payloadBytes: func(txo *types.TransactionOrder) ([]byte, error) { return payload, nil },
	}
	txo := &types.TransactionOrder{}

	// valid data, the P2pkh256.Execute should not return any error
	for i := 0; i < b.N; i++ {
		if res, err := p2pkh256_Execute(pubKeyHash, ownerProof, txo, execEnv); err != nil {
			b.Error(err.Error())
			if !res {
				b.Error("evaluated to false")
			}
		}
	}
}
