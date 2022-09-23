package genesis

import (
	"fmt"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

func TestGenesisRootRecord_FindPubKeyById(t *testing.T) {
	type fields struct {
		RootValidators []*PublicKeyInfo
	}
	type args struct {
		id string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *PublicKeyInfo
	}{
		{
			name: "The nil test, do not crash",
			fields: fields{
				RootValidators: nil,
			},
			args: args{"test"},
			want: nil,
		},
		{
			name: "The empty list, do not crash",
			fields: fields{
				RootValidators: []*PublicKeyInfo{},
			},
			args: args{"test"},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				RootValidators: tt.fields.RootValidators,
				Consensus:      nil,
			}
			if got := x.FindPubKeyById(tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindPubKeyById() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenesisRootRecord_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	_, verifier2 := testsig.CreateSignerAndVerifier(t)
	pubKey2, err := verifier2.MarshalPublicKey()
	require.NoError(t, err)
	consensus := &ConsensusParams{
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       hashAlgo,
	}
	err = consensus.Sign("test", signer)
	require.NoError(t, err)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
		RootValidators []*PublicKeyInfo
		Consensus      *ConsensusParams
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "Root validators nil",
			fields: fields{
				RootValidators: nil,
				Consensus:      consensus,
			},
			wantErr: ErrNoRootValidators,
		},
		{
			name: "Consensus nil",
			fields: fields{
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
				Consensus:      nil},
			wantErr: ErrConsensusIsNil,
		},
		{
			name: "Not signed by validator",
			fields: fields{
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
				Consensus:      &ConsensusParams{TotalRootValidators: totalNodes, BlockRateMs: blockRate, HashAlgorithm: hashAlgo}},
			wantErr: ErrConsensusNotSigned,
		},
		{
			name: "Not signed by all validators",
			fields: fields{
				RootValidators: []*PublicKeyInfo{
					{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey},
					{NodeIdentifier: "xxx", SigningPublicKey: pubKey2, EncryptionPublicKey: pubKey2},
				},
				Consensus: consensus},
			wantErr: ErrConsensusIsNotSignedByAll,
		},
		{
			name: "Unknown validator",
			fields: fields{
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "t", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
				Consensus:      consensus},
			wantErr: "Consensus signed by unknown validator:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				RootValidators: tt.fields.RootValidators,
				Consensus:      tt.fields.Consensus,
			}
			require.ErrorContains(t, x.IsValid(), tt.wantErr)
		})
	}
}

func TestGenesisRootRecord_IsValidMissingPublicKeyInfo(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	signer2, _ := testsig.CreateSignerAndVerifier(t)
	consensus := &ConsensusParams{
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       hashAlgo,
	}
	err := consensus.Sign("test1", signer)
	require.NoError(t, err)
	err = consensus.Sign("test2", signer2)
	require.NoError(t, err)
	pubKey, err := verifier.MarshalPublicKey()
	x := &GenesisRootRecord{
		RootValidators: []*PublicKeyInfo{{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
		Consensus:      consensus,
	}
	require.ErrorContains(t, x.IsValid(), "Consensus signed by unknown validator")
}

// Must be signed by total root validators as specified by consensus structure
func TestGenesisRootRecord_VerifyOk(t *testing.T) {
	consensus := &ConsensusParams{
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       hashAlgo,
	}
	pubKeyInfo := make([]*PublicKeyInfo, totalNodes)
	for i := range pubKeyInfo {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		err := consensus.Sign(fmt.Sprint(i), signer)
		require.NoError(t, err)
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pubKeyInfo[i] = &PublicKeyInfo{NodeIdentifier: fmt.Sprint(i), SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}
	}
	x := &GenesisRootRecord{
		RootValidators: pubKeyInfo,
		Consensus:      consensus,
	}
	require.NoError(t, x.Verify())
}

func TestGenesisRootRecord_Verify(t *testing.T) {
	consensus := &ConsensusParams{
		TotalRootValidators: totalNodes + 1,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       hashAlgo,
	}
	pubKeyInfo := make([]*PublicKeyInfo, totalNodes)
	for i := range pubKeyInfo {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		err := consensus.Sign(fmt.Sprint(i), signer)
		require.NoError(t, err)
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pubKeyInfo[i] = &PublicKeyInfo{NodeIdentifier: fmt.Sprint(i), SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}
	}

	type fields struct {
		RootValidators []*PublicKeyInfo
		Consensus      *ConsensusParams
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "Root validators nil",
			fields: fields{
				RootValidators: nil,
				Consensus:      consensus,
			},
			wantErr: ErrNoRootValidators,
		},
		{
			name: "Consensus nil",
			fields: fields{
				RootValidators: pubKeyInfo,
				Consensus:      nil},
			wantErr: ErrConsensusIsNil,
		},
		{
			name: "Consensus total validators does not match public keys",
			fields: fields{
				RootValidators: pubKeyInfo,
				Consensus:      consensus},
			wantErr: ErrRootValidatorsSize,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				RootValidators: tt.fields.RootValidators,
				Consensus:      tt.fields.Consensus,
			}
			require.ErrorContains(t, x.Verify(), tt.wantErr)
		})
	}
}
