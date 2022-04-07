package genesis

import (
	gocrypto "crypto"
	"strings"
	"testing"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

func TestPartitionGenesis_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
		SystemDescriptionRecord *SystemDescriptionRecord
		Certificate             *certificates.UnicityCertificate
		TrustBase               []byte
	}
	type args struct {
		verifier      crypto.Verifier
		hashAlgorithm gocrypto.Hash
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
			fields:  fields{},
			wantErr: ErrVerifierIsNil,
		},
		{
			name:    "system description record is nil",
			args:    args{verifier: verifier},
			fields:  fields{},
			wantErr: ErrSystemDescriptionIsNil,
		},
		{
			name: "invalid trust base",
			args: args{verifier: verifier},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase: []byte{0},
			},
			wantErrStr: "invalid trust base",
		},
		{
			name: "certificate is nil",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase:   pubKey,
				Certificate: nil,
			},
			wantErr: certificates.ErrUnicityCertificateIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionGenesis{
				SystemDescriptionRecord: tt.fields.SystemDescriptionRecord,
				Certificate:             tt.fields.Certificate,
				TrustBase:               tt.fields.TrustBase,
			}
			err := x.IsValid(tt.args.verifier, tt.args.hashAlgorithm)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.wantErrStr))
			}
		})
	}
}

func TestPartitionGenesis_IsValid_Nil(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	var pr *PartitionGenesis
	require.ErrorIs(t, pr.IsValid(verifier, gocrypto.SHA256), ErrPartitionGenesisIsNil)
}
