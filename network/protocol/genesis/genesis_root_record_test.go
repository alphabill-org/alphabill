package genesis

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
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
				Version:        1,
				RootValidators: tt.fields.RootValidators,
				Consensus:      nil,
			}
			if got := x.FindPubKeyById(tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindPubKeyById() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenesisRootRecord_FindPubKeyById_Nil(t *testing.T) {
	var rg *GenesisRootRecord = nil
	require.Nil(t, rg.FindPubKeyById("test"))
	pubKeyInfo := make([]*PublicKeyInfo, totalNodes)
	for i := range pubKeyInfo {
		_, verifier := testsig.CreateSignerAndVerifier(t)
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pubKeyInfo[i] = &PublicKeyInfo{NodeIdentifier: fmt.Sprint(i), SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}
	}
	rg = &GenesisRootRecord{
		RootValidators: pubKeyInfo,
	}
	require.NotNil(t, rg.FindPubKeyById("1"))
	require.Nil(t, rg.FindPubKeyById("5"))
}

func TestGenesisRootRecord_IsValid_Nil(t *testing.T) {
	var rg *GenesisRootRecord = nil
	err := rg.IsValid()
	require.ErrorIs(t, err, ErrGenesisRootIssNil)
}

func TestGenesisRootRecord_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	_, verifier2 := testsig.CreateSignerAndVerifier(t)
	pubKey2, err := verifier2.MarshalPublicKey()
	require.NoError(t, err)
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
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
			wantErr: ErrNoRootValidators.Error(),
		},
		{
			name: "Consensus nil",
			fields: fields{
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
				Consensus:      nil},
			wantErr: ErrConsensusIsNil.Error(),
		},
		{
			name: "Consensus not valid",
			fields: fields{
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
				Consensus: &ConsensusParams{
					Version:             1,
					TotalRootValidators: totalNodes,
					BlockRateMs:         0,
					ConsensusTimeoutMs:  DefaultConsensusTimeout,
					HashAlgorithm:       hashAlgo}},
			wantErr: "block rate too small",
		},
		{
			name: "Not signed by validator",
			fields: fields{
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
				Consensus: &ConsensusParams{
					Version:             1,
					TotalRootValidators: totalNodes,
					BlockRateMs:         blockRate,
					ConsensusTimeoutMs:  DefaultConsensusTimeout,
					HashAlgorithm:       hashAlgo}},
			wantErr: "consensus parameters is not signed by all validators",
		},
		{
			name: "Not signed by all validators",
			fields: fields{
				RootValidators: []*PublicKeyInfo{
					{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey},
					{NodeIdentifier: "xxx", SigningPublicKey: pubKey2, EncryptionPublicKey: pubKey2},
				},
				Consensus: consensus},
			wantErr: "consensus parameters is not signed by all validators",
		},
		{
			name: "Duplicate validators",
			fields: fields{
				RootValidators: []*PublicKeyInfo{
					{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey},
					{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey},
					{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey},
					{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey},
				},
				Consensus: consensus},
			wantErr: "duplicated node id: test",
		},
		{
			name: "Unknown validator",
			fields: fields{
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "t", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
				Consensus:      consensus},
			wantErr: "consensus parameters signed by unknown validator:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				Version:        1,
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
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	err := consensus.Sign("test1", signer)
	require.NoError(t, err)
	err = consensus.Sign("test2", signer2)
	require.NoError(t, err)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	x := &GenesisRootRecord{
		Version:        1,
		RootValidators: []*PublicKeyInfo{{NodeIdentifier: "test", SigningPublicKey: pubKey, EncryptionPublicKey: pubKey}},
		Consensus:      consensus,
	}
	require.ErrorContains(t, x.IsValid(), "consensus parameters signed by unknown validator")
}

func TestGenesisRootRecord_Verify_Nil(t *testing.T) {
	var rg *GenesisRootRecord = nil
	err := rg.Verify()
	require.ErrorIs(t, err, ErrGenesisRootIssNil)
}

// Must be signed by total root validators as specified by consensus structure
func TestGenesisRootRecord_VerifyOk(t *testing.T) {
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
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
	var x *GenesisRootRecord
	t.Run("Verify", func(t *testing.T) {
		x = &GenesisRootRecord{
			Version:        1,
			RootValidators: pubKeyInfo,
			Consensus:      consensus,
		}
		require.NoError(t, x.Verify())
	})

	t.Run("Serialize and verify", func(t *testing.T) {
		bs, err := types.Cbor.Marshal(x)
		require.NoError(t, err)
		x2 := &GenesisRootRecord{}
		err = types.Cbor.Unmarshal(bs, x2)
		require.NoError(t, err)
		require.NoError(t, x2.Verify())
	})
}

func TestGenesisRootRecord_VerifyErrNoteSignedByAll(t *testing.T) {
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
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
	// remove one signature
	delete(consensus.Signatures, pubKeyInfo[0].NodeIdentifier)
	x := &GenesisRootRecord{
		Version:        1,
		RootValidators: pubKeyInfo,
		Consensus:      consensus,
	}
	require.ErrorContains(t, x.Verify(), "not signed by all")
}

func TestGenesisRootRecord_Verify(t *testing.T) {
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes + 1,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
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
			wantErr: ErrNoRootValidators.Error(),
		},
		{
			name: "Consensus nil",
			fields: fields{
				RootValidators: pubKeyInfo,
				Consensus:      nil},
			wantErr: ErrConsensusIsNil.Error(),
		},
		{
			name: "Consensus total validators does not match public keys",
			fields: fields{
				RootValidators: pubKeyInfo,
				Consensus:      consensus},
			wantErr: ErrRootValidatorsSize.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				Version:        1,
				RootValidators: tt.fields.RootValidators,
				Consensus:      tt.fields.Consensus,
			}
			require.ErrorContains(t, x.Verify(), tt.wantErr)
		})
	}
}
