package certificates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

var zeroHash = make([]byte, 32)

func TestUnicitySeal_IsValid(t *testing.T) {
	_, verifier := generateSigner(t)

	tests := []struct {
		name     string
		seal     *UnicitySeal
		verifier crypto.Verifier
		wantErr  error
	}{
		{
			name:     "seal is nil",
			seal:     nil,
			verifier: verifier,
			wantErr:  ErrUnicitySealIsNil,
		},
		{
			name:     "verifier is nil",
			seal:     &UnicitySeal{},
			verifier: nil,
			wantErr:  ErrUnicitySealVerifierIsNil,
		},
		{
			name: "PreviousHash is nil",
			seal: &UnicitySeal{
				RootChainBlockNumber: 1,
				PreviousHash:         nil,
				Hash:                 zeroHash,
				Signature:            zeroHash,
			},
			verifier: verifier,
			wantErr:  ErrUnicitySealPreviousHashIsNil,
		},
		{
			name: "Hash is nil",
			seal: &UnicitySeal{
				RootChainBlockNumber: 1,
				PreviousHash:         zeroHash,
				Hash:                 nil,
				Signature:            zeroHash,
			},
			verifier: verifier,
			wantErr:  ErrUnicitySealHashIsNil,
		},
		{
			name: "Signature is nil",
			seal: &UnicitySeal{
				RootChainBlockNumber: 1,
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				Signature:            nil,
			},
			verifier: verifier,
			wantErr:  ErrUnicitySealSignatureIsNil,
		},
		{
			name: "block number is invalid is nil",
			seal: &UnicitySeal{
				RootChainBlockNumber: 0,
				PreviousHash:         zeroHash,
				Hash:                 zeroHash,
				Signature:            nil,
			},
			verifier: verifier,
			wantErr:  ErrInvalidBlockNumber,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantErr, tt.seal.IsValid(tt.verifier))
		})
	}
}

func generateSigner(t *testing.T) (crypto.Signer, crypto.Verifier) {
	t.Helper()
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)
	return signer, verifier
}

func TestIsValid_InvalidSignature(t *testing.T) {
	_, verifier := generateSigner(t)
	seal := &UnicitySeal{
		RootChainBlockNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signature:            zeroHash,
	}
	err := seal.IsValid(verifier)
	require.True(t, strings.Contains(err.Error(), "invalid unicity seal signature"))
}

func TestSignAndVerify_Ok(t *testing.T) {
	signer, verifier := generateSigner(t)
	seal := &UnicitySeal{
		RootChainBlockNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign(signer)
	require.NoError(t, err)
	err = seal.Verify(verifier)
	require.NoError(t, err)
}
func TestVerify_SignatureIsNil(t *testing.T) {
	_, verifier := generateSigner(t)
	seal := &UnicitySeal{
		RootChainBlockNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Verify(verifier)
	require.True(t, strings.Contains(err.Error(), "invalid unicity seal signature"))
}

func TestSign_SignerIsNil(t *testing.T) {
	seal := &UnicitySeal{
		RootChainBlockNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign(nil)
	require.Error(t, ErrSignerIsNil, err)
}

func TestVerify_VerifierIsNil(t *testing.T) {
	seal := &UnicitySeal{
		RootChainBlockNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
		Signature:            zeroHash,
	}
	err := seal.Verify(nil)
	require.Error(t, ErrVerifierIsNil, err)
}
