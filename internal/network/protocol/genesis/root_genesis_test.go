package genesis

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"

	"github.com/alphabill-org/alphabill/internal/crypto"

	"github.com/stretchr/testify/require"
)

func TestRootGenesis_IsValid1(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	rootKeyInfo := &PublicKeyInfo{
		NodeIdentifier:      "1",
		SigningPublicKey:    pubKey,
		EncryptionPublicKey: pubKey,
	}
	rootConsensus := &ConsensusParams{
		TotalRootValidators: 1,
		BlockRateMs:         900,
		ConsensusTimeoutMs:  nil,
		QuorumThreshold:     nil,
		HashAlgorithm:       uint32(gocrypto.SHA256),
		Signatures:          make(map[string][]byte),
	}
	err = rootConsensus.Sign("1", signer)
	require.NoError(t, err)
	type fields struct {
		Root          *GenesisRootRecord
		Partitions    []*GenesisPartitionRecord
		HashAlgorithm uint32
	}
	type args struct {
		verifier crypto.Verifier
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name:    "verifier is nil",
			args:    args{verifier: nil},
			wantErr: ErrVerifierIsNil,
		},
		{
			name: "invalid root validator info",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					RootValidators: []*PublicKeyInfo{{NodeIdentifier: "111", SigningPublicKey: nil, EncryptionPublicKey: nil}},
					Consensus:      rootConsensus},
			},
			wantErr: ErrPubKeyInfoSigningKeyIsInvalid,
		},
		{
			name: "partitions not found",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					RootValidators: []*PublicKeyInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: nil,
			},
			wantErr: ErrPartitionsNotFound,
		},
		{
			name: "genesis partition record is nil",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					RootValidators: []*PublicKeyInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: []*GenesisPartitionRecord{nil},
			},
			wantErr: ErrGenesisPartitionRecordIsNil.Error(),
		},
		{
			name: "genesis partition record duplicate system id",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					RootValidators: []*PublicKeyInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: []*GenesisPartitionRecord{
					{
						Nodes:                   []*PartitionNode{{NodeIdentifier: "1", SigningPublicKey: nil, EncryptionPublicKey: nil, BlockCertificationRequest: nil, T2Timeout: 1000}},
						Certificate:             nil,
						SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 1}, T2Timeout: 1000},
					},
					{
						Nodes:                   []*PartitionNode{{NodeIdentifier: "1", SigningPublicKey: nil, EncryptionPublicKey: nil, BlockCertificationRequest: nil, T2Timeout: 1000}},
						Certificate:             nil,
						SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 1}, T2Timeout: 1000},
					},
				},
			},
			wantErr: "duplicated system identifier: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &RootGenesis{
				Root:       tt.fields.Root,
				Partitions: tt.fields.Partitions,
			}
			err := x.IsValid("1", tt.args.verifier)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestRootGenesis_IsValid_Nil(t *testing.T) {
	var rg *RootGenesis = nil
	err := rg.IsValid("", nil)
	require.ErrorContains(t, err, ErrRootGenesisIsNil)
}

func TestRootGenesis(t *testing.T) {
	signingKey, _ := testsig.CreateSignerAndVerifier(t)
	_, encryptionPubKey := testsig.CreateSignerAndVerifier(t)
	hash := []byte{2}
	node := createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey)
	rg := &RootGenesis{
		Partitions: []*GenesisPartitionRecord{
			{
				Nodes: []*PartitionNode{node},
				Certificate: &certificates.UnicityCertificate{
					UnicitySeal: &certificates.UnicitySeal{
						RootChainRoundNumber: 1,
						Hash:                 hash,
						RoundCreationTime:    10000,
					},
				},
				SystemDescriptionRecord: systemDescription,
			},
		},
	}
	require.Equal(t, hash, rg.GetRoundHash())
	require.Equal(t, uint64(1), rg.GetRoundNumber())
	require.Equal(t, 1, len(rg.GetPartitionRecords()))
	require.Equal(t,
		&PartitionRecord{
			SystemDescriptionRecord: systemDescription,
			Validators:              []*PartitionNode{node},
		},
		rg.GetPartitionRecords()[0],
	)
}
