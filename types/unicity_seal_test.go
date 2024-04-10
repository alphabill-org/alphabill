package types

import (
	gocrypto "crypto"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, 32)

func TestUnicitySeal_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	t.Run("seal is nil", func(t *testing.T) {
		var seal *UnicitySeal = nil
		v := map[string]crypto.Verifier{"test": verifier}
		require.Error(t, seal.Verify(v), ErrUnicitySealIsNil)
	})
	t.Run("no root nodes", func(t *testing.T) {
		seal := UnicitySeal{}
		require.Error(t, seal.Verify(nil), ErrRootValidatorInfoMissing)
	})
	t.Run("hash is nil", func(t *testing.T) {
		seal := &UnicitySeal{
			RootChainRoundNumber: 1,
			Timestamp:            NewTimestamp(),
			PreviousHash:         zeroHash,
			Hash:                 nil,
			Signatures:           map[string][]byte{"": zeroHash},
		}
		v := map[string]crypto.Verifier{"test": verifier}
		require.Error(t, seal.Verify(v), ErrUnicitySealHashIsNil)
	})
	t.Run("root round is invalid", func(t *testing.T) {
		seal := &UnicitySeal{
			RootChainRoundNumber: 0,
			Timestamp:            NewTimestamp(),
			PreviousHash:         zeroHash,
			Hash:                 zeroHash,
			Signatures:           nil,
		}
		v := map[string]crypto.Verifier{"test": verifier}
		require.Error(t, seal.Verify(v), ErrInvalidRootRound)
	})
	t.Run("timestamp is missing", func(t *testing.T) {
		seal := &UnicitySeal{
			RootChainRoundNumber: 1,
			PreviousHash:         zeroHash,
			Hash:                 zeroHash,
			Signatures:           nil,
		}
		v := map[string]crypto.Verifier{"test": verifier}
		require.Error(t, seal.Verify(v), errInvalidTimestamp)
	})
}

func TestIsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signatures:           map[string][]byte{"test": zeroHash},
	}
	verifiers := map[string]crypto.Verifier{"test": verifier}

	err := seal.Verify(verifiers)
	require.True(t, strings.Contains(err.Error(), "invalid unicity seal signature"))
}

func TestSignAndVerify_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign("test", signer)
	require.NoError(t, err)
	verifiers := map[string]crypto.Verifier{"test": verifier}
	err = seal.Verify(verifiers)
	require.NoError(t, err)
}
func TestVerify_SignatureIsNil(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	verifiers := map[string]crypto.Verifier{"test": verifier}
	err := seal.Verify(verifiers)
	require.EqualError(t, err, "unicity seal validation error: no signatures")
}

func TestVerify_SignatureUnknownSigner(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signatures:           map[string][]byte{"test": zeroHash},
	}
	verifiers := map[string]crypto.Verifier{"xxx": verifier}
	err := seal.Verify(verifiers)
	require.ErrorIs(t, err, ErrUnknownSigner)
}

func TestSign_SignerIsNil(t *testing.T) {
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign("test", nil)
	require.ErrorIs(t, err, ErrSignerIsNil)
}

func TestVerify_VerifierIsNil(t *testing.T) {
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signatures:           map[string][]byte{"": zeroHash},
	}
	err := seal.Verify(nil)
	require.ErrorIs(t, err, ErrRootValidatorInfoMissing)
}

func Test_NewTimestamp(t *testing.T) {
	require.NotZero(t, NewTimestamp())
}

func TestSignatureMap_Serialize(t *testing.T) {
	t.Run("SignatureMap is empty", func(t *testing.T) {
		smap := SignatureMap{}
		data, err := smap.MarshalCBOR()
		require.NoError(t, err)
		res := SignatureMap{}
		require.NoError(t, res.UnmarshalCBOR(data))
		require.Empty(t, smap)
	})
	t.Run("SignatureMap normal", func(t *testing.T) {
		smap := SignatureMap{"x": []byte{9, 9, 9}, "1": []byte{1, 2, 3}, "a": []byte{0, 0, 0}, "2": []byte{2, 3, 4}}
		data, err := smap.MarshalCBOR()
		require.NoError(t, err)
		res := SignatureMap{}
		require.NoError(t, res.UnmarshalCBOR(data))
		require.EqualValues(t, smap, res)
	})
}

func TestSignatureMap_AddToHasher_Nil(t *testing.T) {
	var smap SignatureMap
	hasher := gocrypto.SHA256.New()
	smap.AddToHasher(hasher)
	require.Nil(t, smap)
}

func TestSeal_AddToHasher(t *testing.T) {
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            100000,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signatures:           map[string][]byte{"xxx": {1, 1, 1}, "aaa": {2, 2, 2}},
	}
	hasher := gocrypto.SHA256.New()
	seal.AddToHasher(hasher)
	hash := hasher.Sum(nil)
	// serialize manually
	hasher.Reset()
	hasher.Write(util.Uint64ToBytes(seal.RootChainRoundNumber))
	hasher.Write(util.Uint64ToBytes(seal.Timestamp))
	hasher.Write(seal.PreviousHash)
	hasher.Write(seal.Hash)
	// add signatures, in lexical order
	hasher.Write([]byte("aaa"))
	hasher.Write([]byte{2, 2, 2})
	hasher.Write([]byte("xxx"))
	hasher.Write([]byte{1, 1, 1})
	require.Equal(t, hash, hasher.Sum(nil))
}
