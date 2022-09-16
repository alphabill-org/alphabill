package genesis

import (
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const (
	totalNodes              = 4
	blockRate               = 900
	consensusTimeout uint32 = 10000
	quorum           uint32 = 3
	hashAlgo                = uint32(gocrypto.SHA256)
)

func TestConsensusParams_HashSignaturesAreIgnored(t *testing.T) {
	testConsensusTimeout := consensusTimeout
	testQuorum := quorum

	x := &ConsensusParams{
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  &testConsensusTimeout,
		QuorumThreshold:     &testQuorum,
		HashAlgorithm:       uint32(gocrypto.SHA256),
		Signatures:          make(map[string][]byte),
	}
	// calc hash
	cBytes := x.Bytes()
	// modify signatures
	x.Signatures["1"] = []byte{0, 1}
	// hash must still be the same
	require.Equal(t, cBytes, x.Bytes())
}

// Probably too pointless, maybe remove
func TestConsensusParams_HashFieldsIncluded(t *testing.T) {
	testConsensusTimeout := consensusTimeout
	testQuorum := quorum

	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  &testConsensusTimeout,
		QuorumThreshold:     &testQuorum,
		HashAlgorithm:       hashAlgo,
		Signatures:          make(map[string][]byte),
	}
	// calc hash
	cBytes := x.Bytes()

	type fields struct {
		TotalRootValidators uint32
		BlockRateMs         uint32
		ConsensusTimeoutMs  uint32
		QuorumThreshold     uint32
		HashAlgorithm       uint32
		Signatures          map[string][]byte
	}
	tests := []struct {
		name   string
		refB   []byte
		fields fields
	}{
		{
			name: "Different total nodes",
			refB: cBytes,
			fields: fields{
				TotalRootValidators: 1,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
		},
		{
			name: "Different block rate",
			refB: cBytes,
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         1000,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
		},
		{
			name: "Different consensus timeout",
			refB: cBytes,
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  5000,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
		},
		{
			name: "Different quorum",
			refB: cBytes,
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     4,
				HashAlgorithm:       hashAlgo},
		},
		{
			name: "Different hash",
			refB: cBytes,
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &ConsensusParams{
				TotalRootValidators: tt.fields.TotalRootValidators,
				BlockRateMs:         tt.fields.BlockRateMs,
				ConsensusTimeoutMs:  &tt.fields.ConsensusTimeoutMs,
				QuorumThreshold:     &tt.fields.QuorumThreshold,
				HashAlgorithm:       tt.fields.HashAlgorithm,
				Signatures:          tt.fields.Signatures,
			}
			// require hash not equal
			require.NotEqual(t, cBytes, x.Bytes())
		})
	}
}

func TestConsensusParams_IsValid(t *testing.T) {
	type fields struct {
		TotalRootValidators uint32
		BlockRateMs         uint32
		ConsensusTimeoutMs  uint32
		QuorumThreshold     uint32
		HashAlgorithm       uint32
		Signatures          map[string][]byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name: "Total root nodes 0",
			fields: fields{
				TotalRootValidators: 0,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidNumberOfRootValidators,
		},
		{
			name: "Total root nodes 3",
			fields: fields{
				TotalRootValidators: 3,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidNumberOfRootValidators,
		},
		{
			name: "Unknown hash algorithm",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       666},
			wantErr: ErrUnknownHashAlgorithm,
		},
		{
			name: "Quorum too low",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     1,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidQuorumThreshold,
		},
		{
			name: "Quorum too high",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     1,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidQuorumThreshold,
		},
		{
			name: "Invalid consensus timeout",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  blockRate - 1,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidConsensusTimeout,
		},
		{
			name: "Invalid block rate",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         10,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrBlockRateTooSmall,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &ConsensusParams{
				TotalRootValidators: tt.fields.TotalRootValidators,
				BlockRateMs:         tt.fields.BlockRateMs,
				ConsensusTimeoutMs:  &tt.fields.ConsensusTimeoutMs,
				QuorumThreshold:     &tt.fields.QuorumThreshold,
				HashAlgorithm:       tt.fields.HashAlgorithm,
				Signatures:          tt.fields.Signatures,
			}
			require.Equal(t, tt.wantErr, x.IsValid())
		})
	}
}

// not a too useful test
func TestGetMinQuorumThreshold(t *testing.T) {
	type args struct {
		totalRootValidators uint32
	}
	tests := []struct {
		name string
		args args
		want uint32
	}{
		{
			name: "total number 1",
			args: args{totalRootValidators: 1},
			want: 1,
		},
		{
			name: "total number 2",
			args: args{totalRootValidators: 2},
			want: 0,
		},
		{
			name: "total number 4",
			args: args{totalRootValidators: 4},
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetMinQuorumThreshold(tt.args.totalRootValidators); got != tt.want {
				t.Errorf("GetMinQuorumThreshold() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConsensusSignAndVerify_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
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
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       hashAlgo,
		Signatures:          nil,
	}
	verifiers := map[string]crypto.Verifier{"test": ver}
	err := x.Verify(verifiers)
	require.ErrorIs(t, err, ErrConsensusNotSigned)
}

func TestConsensusIsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string][]byte{"test": {0, 0}},
	}

	verifiers := map[string]crypto.Verifier{"test": verifier}

	err := x.Verify(verifiers)
	require.True(t, strings.Contains(err.Error(), "invalid consensus signature"))
}

func TestConsensusVerify_UnknownSigner(t *testing.T) {
	_, ver := testsig.CreateSignerAndVerifier(t)
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string][]byte{"test": {0, 0}},
	}
	verifiers := map[string]crypto.Verifier{"t": ver}
	err := x.Verify(verifiers)
	require.ErrorContains(t, err, "Consensus signed by unknown validator:")
}

func TestSign_SignerIsNil(t *testing.T) {
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		HashAlgorithm:       hashAlgo,
	}
	err := x.Sign("test", nil)
	require.ErrorIs(t, err, certificates.ErrSignerIsNil)
}

func TestVerify_VerifierIsNil(t *testing.T) {
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string][]byte{"test": {0, 0}},
	}
	err := x.Verify(nil)
	require.ErrorIs(t, err, certificates.ErrRootValidatorInfoMissing)
}
