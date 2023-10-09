package types

import (
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, 32)

func TestUnicitySeal_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)

	tests := []struct {
		name     string
		seal     *UnicitySeal
		verifier map[string]crypto.Verifier
		wantErr  error
	}{
		{
			name:     "seal is nil",
			seal:     nil,
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrUnicitySealIsNil,
		},
		{
			name:     "no root nodes",
			seal:     &UnicitySeal{},
			verifier: nil,
			wantErr:  ErrRootValidatorInfoMissing,
		},
		{
			name: "Hash is nil",
			seal: &UnicitySeal{
				RootChainRoundNumber: 1,
				Timestamp:            util.MakeTimestamp(),
				PreviousHash:         zeroHash,
				Hash:                 nil,
				Signatures:           map[string][]byte{"": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrUnicitySealHashIsNil,
		},
		{
			name: "Signature is nil",
			seal: &UnicitySeal{
				RootChainRoundNumber: 1,
				Timestamp:            util.MakeTimestamp(),
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				Signatures:           nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrUnicitySealSignatureIsNil,
		},
		{
			name: "block number is invalid is nil",
			seal: &UnicitySeal{
				RootChainRoundNumber: 0,
				Timestamp:            util.MakeTimestamp(),
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				Signatures:           nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrInvalidBlockNumber,
		},
		{
			name: "Timestamp is missing",
			seal: &UnicitySeal{
				RootChainRoundNumber: 1,
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				Signatures:           nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  errInvalidTimestamp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantErr, tt.seal.IsValid(tt.verifier))
		})
	}
}

func TestIsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            util.MakeTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signatures:           map[string][]byte{"test": zeroHash},
	}
	verifiers := map[string]crypto.Verifier{"test": verifier}

	err := seal.IsValid(verifiers)
	require.True(t, strings.Contains(err.Error(), "invalid unicity seal signature"))
}

func TestSignAndVerify_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            util.MakeTimestamp(),
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
		Timestamp:            util.MakeTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	verifiers := map[string]crypto.Verifier{"test": verifier}
	err := seal.Verify(verifiers)
	require.ErrorIs(t, err, errUnicitySealNoSignature)
}

func TestVerify_SignatureUnknownSigner(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            util.MakeTimestamp(),
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
		Timestamp:            util.MakeTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign("test", nil)
	require.ErrorIs(t, err, ErrSignerIsNil)
}

func TestVerify_VerifierIsNil(t *testing.T) {
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            util.MakeTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signatures:           map[string][]byte{"": zeroHash},
	}
	err := seal.Verify(nil)
	require.ErrorIs(t, err, ErrRootValidatorInfoMissing)
}
