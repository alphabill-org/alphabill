package certificates

import (
	"strings"
	"testing"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
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
			name:     "no root validators",
			seal:     &UnicitySeal{},
			verifier: nil,
			wantErr:  ErrRootValidatorInfoMissing,
		},
		{
			name: "PreviousHash is nil",
			seal: &UnicitySeal{
				RootChainRoundNumber: 1,
				PreviousHash:         nil,
				Hash:                 zeroHash,
				RoundCreationTime:    1,
				Signatures:           map[string][]byte{"": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrUnicitySealPreviousHashIsNil,
		},
		{
			name: "Hash is nil",
			seal: &UnicitySeal{
				RootChainRoundNumber: 1,
				PreviousHash:         zeroHash,
				Hash:                 nil,
				RoundCreationTime:    1,
				Signatures:           map[string][]byte{"": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrUnicitySealHashIsNil,
		},
		{
			name: "Signature is nil",
			seal: &UnicitySeal{
				RootChainRoundNumber: 1,
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				RoundCreationTime:    1,
				Signatures:           nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrUnicitySealSignatureIsNil,
		},
		{
			name: "Round creation time is 0",
			seal: &UnicitySeal{
				RootChainRoundNumber: 1,
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				RoundCreationTime:    0,
				Signatures:           map[string][]byte{"test": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrRoundCreationTimeNotSet,
		},
		{
			name: "block number is invalid is nil",
			seal: &UnicitySeal{
				RootChainRoundNumber: 0,
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				RoundCreationTime:    1,
				Signatures:           nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrInvalidBlockNumber,
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
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		RoundCreationTime:    1,
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
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		RoundCreationTime:    1,
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
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		RoundCreationTime:    1,
	}
	verifiers := map[string]crypto.Verifier{"test": verifier}
	err := seal.Verify(verifiers)
	require.True(t, strings.Contains(err.Error(), "invalid unicity seal signature"))
}

func TestVerify_SignatureUnknownSigner(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		RoundCreationTime:    1,
		Signatures:           map[string][]byte{"test": zeroHash},
	}
	verifiers := map[string]crypto.Verifier{"xxx": verifier}
	err := seal.Verify(verifiers)
	require.ErrorIs(t, err, ErrUnknownSigner)
}

func TestSign_SignerIsNil(t *testing.T) {
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		RoundCreationTime:    1,
	}
	err := seal.Sign("test", nil)
	require.ErrorIs(t, err, ErrSignerIsNil)
}

func TestVerify_VerifierIsNil(t *testing.T) {
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		RoundCreationTime:    1,
		Signatures:           map[string][]byte{"": zeroHash},
	}
	err := seal.Verify(nil)
	require.ErrorIs(t, err, ErrRootValidatorInfoMissing)
}
