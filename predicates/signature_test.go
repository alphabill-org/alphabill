package predicates

import (
	"errors"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
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
		require.EqualError(t, err, `decoding owner proof as Signature: cbor: cannot unmarshal positive integer into Go value of type predicates.P2pkh256Signature`)
		require.Nil(t, pk)
	})

	t.Run("success", func(t *testing.T) {
		pubKey := []byte{0x2, 0x12, 0x91, 0x1c, 0x73, 0x41, 0x39, 0x9e, 0x87, 0x68, 0x0, 0xa2, 0x68, 0x85, 0x5c, 0x89, 0x4c, 0x43, 0xeb, 0x84, 0x9a, 0x72, 0xac, 0x5a, 0x9d, 0x26, 0xa0, 0x9, 0x10, 0x41, 0xc1, 0x7, 0xf0}
		ownerProof, err := EncodeSignature([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}, pubKey)
		require.NoError(t, err)
		pk, err := ExtractPubKey(ownerProof)
		require.NoError(t, err)
		require.Equal(t, pubKey, pk)
	})
}

func Test_OwnerProofer(t *testing.T) {
	t.Run("nil signer crashes on proofer call", func(t *testing.T) {
		// sending nil signer doesn't fail until attempting to use the proofer
		proofer := OwnerProofer(nil, []byte{1, 2, 3})
		require.NotNil(t, proofer)
		// panic: runtime error: invalid memory address or nil pointer dereference
		require.Panics(t, func() { proofer([]byte{4, 5, 6}) })
	})

	t.Run("signer returns error", func(t *testing.T) {
		expErr := errors.New("won't sign this!")
		ms := mockSigner{signBytes: func(b []byte) ([]byte, error) { return nil, expErr }}

		proofer := OwnerProofer(ms, []byte{1, 2, 3})
		require.NotNil(t, proofer)

		op, err := proofer([]byte{1, 1, 1})
		require.ErrorIs(t, err, expErr)
		require.Nil(t, op)
	})

	t.Run("success", func(t *testing.T) {
		data := []byte{0xD, 0xA, 0x7, 0xA}
		signature := []byte{2, 2, 2, 2}
		pubKey := []byte{3, 3, 3, 3}
		ms := mockSigner{
			signBytes: func(b []byte) ([]byte, error) {
				require.Equal(t, data, b)
				return signature, nil
			}}

		proofer := OwnerProofer(ms, pubKey)
		require.NotNil(t, proofer)

		ownerProof, err := proofer(data)
		require.NoError(t, err)
		sig := P2pkh256Signature{}
		require.NoError(t, cbor.Unmarshal(ownerProof, &sig))
		require.Equal(t, signature, sig.Sig)
		require.Equal(t, pubKey, sig.PubKey)
	})
}

func Test_OwnerProoferForSigner(t *testing.T) {
	t.Run("nil signer panics", func(t *testing.T) {
		// nil signer panics right away (verifier is requested)
		// panic: runtime error: invalid memory address or nil pointer dereference
		require.Panics(t, func() { OwnerProoferForSigner(nil) })
	})

	t.Run("nil verifier is returned", func(t *testing.T) {
		ms := mockSigner{
			verifier: func() (abcrypto.Verifier, error) { return nil, nil },
		}
		// signer returns nil verifier, panics right away
		// panic: runtime error: invalid memory address or nil pointer dereference
		require.Panics(t, func() { OwnerProoferForSigner(ms) })
	})

	t.Run("asking verifier returns error", func(t *testing.T) {
		expErr := errors.New("no verifier here")
		ms := mockSigner{
			verifier: func() (abcrypto.Verifier, error) { return nil, expErr },
		}
		proofer := OwnerProoferForSigner(ms)
		require.NotNil(t, proofer)

		op, err := proofer([]byte{0xD, 0xA, 0x7, 0xA})
		require.ErrorIs(t, err, expErr)
		require.Nil(t, op)
	})

	t.Run("verifier fails to marshal public key", func(t *testing.T) {
		expErr := errors.New("not giving you my public key")
		ms := mockSigner{
			verifier: func() (abcrypto.Verifier, error) {
				return mockVerifier{getPubKey: func() ([]byte, error) { return nil, expErr }}, nil
			},
		}
		proofer := OwnerProoferForSigner(ms)
		require.NotNil(t, proofer)

		op, err := proofer([]byte{0xD, 0xA, 0x7, 0xA})
		require.ErrorIs(t, err, expErr)
		require.Nil(t, op)
	})

	t.Run("success", func(t *testing.T) {
		data := []byte{0xD, 0xA, 0x7, 0xA}
		signature := []byte{2, 2, 2, 2}
		pubKey := []byte{3, 3, 3, 3}
		ms := mockSigner{
			verifier: func() (abcrypto.Verifier, error) {
				return mockVerifier{getPubKey: func() ([]byte, error) { return pubKey, nil }}, nil
			},
			signBytes: func(b []byte) ([]byte, error) {
				require.Equal(t, data, b)
				return signature, nil
			}}

		proofer := OwnerProoferForSigner(ms)
		require.NotNil(t, proofer)

		ownerProof, err := proofer(data)
		require.NoError(t, err)
		sig := P2pkh256Signature{}
		require.NoError(t, cbor.Unmarshal(ownerProof, &sig))
		require.Equal(t, signature, sig.Sig)
		require.Equal(t, pubKey, sig.PubKey)
	})
}

func Test_OwnerProoferSecp256K1(t *testing.T) {
	t.Run("invalid private key", func(t *testing.T) {
		proofer := OwnerProoferSecp256K1([]byte{0, 0, 0, 0}, []byte{1, 1, 1, 1})
		require.NotNil(t, proofer)

		op, err := proofer([]byte{1, 1, 1})
		require.EqualError(t, err, `creating signer with given private key: invalid private key length. Is 4 (expected 32)`)
		require.Nil(t, op)
	})

	t.Run("success", func(t *testing.T) {
		pubKey := []byte{0x2, 0x12, 0x91, 0x1c, 0x73, 0x41, 0x39, 0x9e, 0x87, 0x68, 0x0, 0xa2, 0x68, 0x85, 0x5c, 0x89, 0x4c, 0x43, 0xeb, 0x84, 0x9a, 0x72, 0xac, 0x5a, 0x9d, 0x26, 0xa0, 0x9, 0x10, 0x41, 0xc1, 0x7, 0xf0}
		privKey := []byte{0xa5, 0xe8, 0xbf, 0xf9, 0x73, 0x3e, 0xbc, 0x75, 0x1a, 0x45, 0xca, 0x4b, 0x8c, 0xc6, 0xce, 0x8e, 0x76, 0xc8, 0x31, 0x6a, 0x5e, 0xb5, 0x56, 0xf7, 0x38, 0x9, 0x2d, 0xf6, 0x23, 0x2e, 0x78, 0xde}

		proofer := OwnerProoferSecp256K1(privKey, pubKey)
		require.NotNil(t, proofer)

		ownerProof, err := proofer([]byte{0xD, 0xA, 0x7, 0xA})
		require.NoError(t, err)
		sig := P2pkh256Signature{}
		require.NoError(t, cbor.Unmarshal(ownerProof, &sig))
		require.NotEmpty(t, sig.Sig)
		require.Equal(t, pubKey, sig.PubKey)
	})
}

func Test_OwnerProofers(t *testing.T) {
	// all variations should return the same binary
	pubKey := []byte{0x2, 0x12, 0x91, 0x1c, 0x73, 0x41, 0x39, 0x9e, 0x87, 0x68, 0x0, 0xa2, 0x68, 0x85, 0x5c, 0x89, 0x4c, 0x43, 0xeb, 0x84, 0x9a, 0x72, 0xac, 0x5a, 0x9d, 0x26, 0xa0, 0x9, 0x10, 0x41, 0xc1, 0x7, 0xf0}
	privKey := []byte{0xa5, 0xe8, 0xbf, 0xf9, 0x73, 0x3e, 0xbc, 0x75, 0x1a, 0x45, 0xca, 0x4b, 0x8c, 0xc6, 0xce, 0x8e, 0x76, 0xc8, 0x31, 0x6a, 0x5e, 0xb5, 0x56, 0xf7, 0x38, 0x9, 0x2d, 0xf6, 0x23, 0x2e, 0x78, 0xde}

	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(privKey)
	require.NoError(t, err)

	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}

	keys, err := OwnerProoferSecp256K1(privKey, pubKey)(data)
	require.NoError(t, err)
	pubk, err := OwnerProofer(signer, pubKey)(data)
	require.NoError(t, err)
	sign, err := OwnerProoferForSigner(signer)(data)
	require.NoError(t, err)

	require.Equal(t, keys, pubk)
	require.Equal(t, keys, sign)
}

type mockSigner struct {
	signBytes func([]byte) ([]byte, error)      // signer.SignBytes
	verifier  func() (abcrypto.Verifier, error) // signer.Verifier
}

func (ms mockSigner) SignBytes(data []byte) ([]byte, error) { return ms.signBytes(data) }
func (ms mockSigner) Verifier() (abcrypto.Verifier, error)  { return ms.verifier() }

type mockVerifier struct {
	abcrypto.Verifier
	getPubKey func() ([]byte, error) // MarshalPublicKey
}

func (mv mockVerifier) MarshalPublicKey() ([]byte, error) { return mv.getPubKey() }
