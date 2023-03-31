package genesis

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

const (
	totalNodes              = 4
	blockRate               = 900
	consensusTimeout uint32 = 10000
	quorum           uint32 = 3
	hashAlgo                = uint32(gocrypto.SHA256)
)

func TestConsensusParams_HashSignaturesAreIgnored(t *testing.T) {
	x := &ConsensusParams{
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  consensusTimeout,
		QuorumThreshold:     quorum,
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
	x := &ConsensusParams{
		TotalRootValidators: 4,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  consensusTimeout,
		QuorumThreshold:     quorum,
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string][]byte{},
	}
	// serialized form
	serialized := []byte{
		0, 0, 0, 4, // total 4 nodes as uint32
		0, 0, 0x03, 0x84, // block rate 900 as uint32
		0, 0, 0x27, 0x10, // local timeout 10000 as uint32
		0, 0, 0, 3, // quorum 3 as uint32
		0, 0, 0, 5, // hash algorithm SHA256 as uint32
	}
	// require hash not equal
	require.Equal(t, serialized, x.Bytes())
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
		wantErr string
	}{
		{
			name: "Total root nodes 0",
			fields: fields{
				TotalRootValidators: 0,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidNumberOfRootValidators.Error(),
		},
		{
			name: "Unknown hash algorithm",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       666},
			wantErr: ErrUnknownHashAlgorithm.Error(),
		},
		{
			name: "Quorum too low",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     1,
				HashAlgorithm:       hashAlgo},
			wantErr: "quorum threshold set too low 1, must be at least 3",
		},
		{
			name: "Quorum too high",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     totalNodes + 1,
				HashAlgorithm:       hashAlgo},
			wantErr: "quorum threshold set higher 5 than number of validators in root chain 4",
		},
		{
			name: "Invalid consensus timeout",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         blockRate,
				ConsensusTimeoutMs:  blockRate - 1,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrInvalidConsensusTimeout.Error(),
		},
		{
			name: "Invalid block rate",
			fields: fields{
				TotalRootValidators: totalNodes,
				BlockRateMs:         10,
				ConsensusTimeoutMs:  consensusTimeout,
				QuorumThreshold:     quorum,
				HashAlgorithm:       hashAlgo},
			wantErr: ErrBlockRateTooSmall.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &ConsensusParams{
				TotalRootValidators: tt.fields.TotalRootValidators,
				BlockRateMs:         tt.fields.BlockRateMs,
				ConsensusTimeoutMs:  tt.fields.ConsensusTimeoutMs,
				QuorumThreshold:     tt.fields.QuorumThreshold,
				HashAlgorithm:       tt.fields.HashAlgorithm,
				Signatures:          tt.fields.Signatures,
			}
			require.ErrorContains(t, x.IsValid(), tt.wantErr)
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
			want: 2,
		},
		{
			name: "total number 4",
			args: args{totalRootValidators: 4},
			want: 3,
		},
		{
			name: "total number 7",
			args: args{totalRootValidators: 7},
			want: 5,
		},
		{
			name: "total number 10",
			args: args{totalRootValidators: 10},
			want: 7,
		},
		{
			name: "total number 30",
			args: args{totalRootValidators: 30},
			want: 21,
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
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		QuorumThreshold:     GetMinQuorumThreshold(4),
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
		QuorumThreshold:     GetMinQuorumThreshold(4),
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
		QuorumThreshold:     GetMinQuorumThreshold(4),
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string][]byte{"test": {0, 0}},
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
		QuorumThreshold:     GetMinQuorumThreshold(4),
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string][]byte{"test": {0, 0}},
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
		QuorumThreshold:     GetMinQuorumThreshold(4),
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
		QuorumThreshold:     GetMinQuorumThreshold(4),
		HashAlgorithm:       hashAlgo,
		Signatures:          map[string][]byte{"test": {0, 0}},
	}
	err := x.Verify(nil)
	require.ErrorContains(t, err, "missing root node public info")
}

func TestConsensusParams_Nil(t *testing.T) {
	var x *ConsensusParams = nil
	require.ErrorIs(t, x.IsValid(), ErrConsensusParamsIsNil)
	require.Empty(t, x.Bytes())
	sig, ver := testsig.CreateSignerAndVerifier(t)
	require.ErrorIs(t, x.Sign("1", sig), ErrConsensusParamsIsNil)
	tb := map[string]crypto.Verifier{"1": ver}
	require.ErrorIs(t, x.Verify(tb), ErrConsensusParamsIsNil)

}
