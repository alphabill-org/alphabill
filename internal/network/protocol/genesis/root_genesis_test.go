package genesis

import (
	gocrypto "crypto"
	"strings"
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
	alg := gocrypto.Hash(rootConsensus.HashAlgorithm)
	hash := rootConsensus.Hash(alg)
	sig, err := signer.SignHash(hash)
	require.NoError(t, err)
	rootConsensus.Signatures["1"] = sig
	type fields struct {
		Root          *GenesisRootRecord
		Partitions    []*GenesisPartitionRecord
		TrustBase     []byte
		HashAlgorithm uint32
	}
	type args struct {
		verifier crypto.Verifier
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErr    error
		wantErrStr string
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
			wantErr: ErrMissingPubKeyInfo,
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
			wantErr: ErrGenesisPartitionRecordIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &RootGenesis{
				Root:       tt.fields.Root,
				Partitions: tt.fields.Partitions,
			}
			err := x.IsValid("1", tt.args.verifier)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.wantErrStr))
			}

		})
	}
}

func TestRootGenesis_IsValid_Nil(t *testing.T) {
	var rg *RootGenesis = nil
	err := rg.IsValid("", nil)
	require.ErrorIs(t, err, ErrRootGenesisIsNil)
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
