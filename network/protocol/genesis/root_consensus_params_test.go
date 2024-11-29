package genesis

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

const (
	totalNodes              = 4
	blockRate               = 900
	consensusTimeout uint32 = 10000
	hashAlgo                = uint32(gocrypto.SHA256)
)

func TestConsensusParams_HashSignaturesAreIgnored(t *testing.T) {
	x := &ConsensusParams{
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  consensusTimeout,
		HashAlgorithm:       uint32(gocrypto.SHA256),
		Signatures:          make(map[string]hex.Bytes),
	}
	// calc hash
	cBytes, err := x.SigBytes()
	require.NoError(t, err)
	// modify signatures
	x.Signatures["1"] = []byte{0, 1}
	// hash must still be the same
	cBytes2, err := x.SigBytes()
	require.NoError(t, err)
	require.Equal(t, cBytes, cBytes2)
}

// Probably too pointless, maybe remove
func TestConsensusParams_HashFieldsIncluded(t *testing.T) {
	x := &ConsensusParams{
		Version:             1,
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  consensusTimeout,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string]hex.Bytes{},
	}
	sigBytes, err := x.SigBytes()
	require.NoError(t, err)

	x.Signatures = nil
	serialized, err := x.MarshalCBOR()
	require.NoError(t, err)

	require.Equal(t, sigBytes, serialized)
}

func TestConsensusParams_IsValid(t *testing.T) {
	type fields struct {
		TotalRootValidators uint32
		BlockRateMs         uint32
		ConsensusTimeoutMs  uint32
		HashAlgorithm       uint32
		Signatures          map[string]hex.Bytes
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "Total root nodes 0",
			fields: fields{
				TotalRootValidators: 0,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidNumberOfRootValidators.Error(),
		},
		{
			name: "Unknown hash algorithm",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				HashAlgorithm:       666},
			wantErr: ErrUnknownHashAlgorithm.Error(),
		},
		{
			name: "Invalid consensus timeout",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  blockRate - 1,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidConsensusTimeout.Error(),
		},
		{
			name: "Invalid block rate",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         10,
				ConsensusTimeoutMs:  consensusTimeout,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrBlockRateTooSmall.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &ConsensusParams{
				Version:             1,
				TotalRootValidators: tt.fields.TotalRootValidators,
				BlockRateMs:         tt.fields.BlockRateMs,
				ConsensusTimeoutMs:  tt.fields.ConsensusTimeoutMs,
				HashAlgorithm:       tt.fields.HashAlgorithm,
				Signatures:          tt.fields.Signatures,
			}
			require.ErrorContains(t, x.IsValid(), tt.wantErr)
		})
	}
}

func TestConsensusSignAndVerify_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	err := x.Sign("test", signer)
	require.NoError(t, err)
	verifiers := map[string]crypto.Verifier{"test": verifier}
	err = x.Verify(verifiers)
	require.NoError(t, err)
}

func TestConsensusVerify_SignatureIsNil(t *testing.T) {
	_, ver := testsig.CreateSignerAndVerifier(t)
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
		Signatures:          nil,
	}
	verifiers := map[string]crypto.Verifier{"test": ver}
	err := x.Verify(verifiers)
	require.ErrorContains(t, err, "consensus parameters is not signed by all validators")
}

func TestConsensusIsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string]hex.Bytes{"test": {0, 0}},
	}

	verifiers := map[string]crypto.Verifier{"test": verifier}

	err := x.Verify(verifiers)
	require.ErrorContains(t, err, "consensus parameters signature verification error")
}

func TestConsensusVerify_UnknownSigner(t *testing.T) {
	_, ver := testsig.CreateSignerAndVerifier(t)
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string]hex.Bytes{"test": {0, 0}},
	}
	verifiers := map[string]crypto.Verifier{"t": ver}
	err := x.Verify(verifiers)
	require.ErrorContains(t, err, "consensus parameters signed by unknown validator:")
}

func TestSign_SignerIsNil(t *testing.T) {
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	err := x.Sign("test", nil)
	require.ErrorContains(t, err, ErrSignerIsNil.Error())
}

func TestVerify_VerifierIsNil(t *testing.T) {
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string]hex.Bytes{"test": {0, 0}},
	}
	err := x.Verify(nil)
	require.ErrorContains(t, err, "missing root node public info")
}

func TestConsensusParams_Nil(t *testing.T) {
	var x *ConsensusParams = nil
	require.ErrorIs(t, x.IsValid(), ErrConsensusParamsIsNil)
	_, err := x.SigBytes()
	require.ErrorIs(t, err, ErrConsensusParamsIsNil)
	sig, ver := testsig.CreateSignerAndVerifier(t)
	require.ErrorIs(t, x.Sign("1", sig), ErrConsensusParamsIsNil)
	tb := map[string]crypto.Verifier{"1": ver}
	require.ErrorIs(t, x.Verify(tb), ErrConsensusParamsIsNil)
}
