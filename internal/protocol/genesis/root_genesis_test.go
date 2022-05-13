package genesis

import (
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"github.com/stretchr/testify/require"
)

func TestRootGenesis_IsValid1(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
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
			name: "invalid trust base",
			args: args{
				verifier: verifier,
			},
			fields:     fields{TrustBase: nil},
			wantErrStr: "invalid trust base",
		},
		{
			name: "partitions not found",
			args: args{
				verifier: verifier,
			},
			fields:  fields{TrustBase: pubKey},
			wantErr: ErrPartitionsNotFound,
		},
		{
			name: "genesis partition record is nil",
			args: args{
				verifier: verifier,
			},
			fields:  fields{TrustBase: pubKey, Partitions: []*GenesisPartitionRecord{nil}},
			wantErr: ErrGenesisPartitionRecordIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &RootGenesis{
				Partitions:    tt.fields.Partitions,
				TrustBase:     tt.fields.TrustBase,
				HashAlgorithm: tt.fields.HashAlgorithm,
			}
			err := x.IsValid(tt.args.verifier)
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
	err := rg.IsValid(nil)
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
