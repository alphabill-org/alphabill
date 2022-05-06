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

	keyInfo := &PublicKeyInfo{
		NodeIdentifier: "1",
		PublicKey:      pubKey,
	}

	type fields struct {
		SystemDescriptionRecord *SystemDescriptionRecord
		Certificate             *certificates.UnicityCertificate
		Keys                    []*PublicKeyInfo
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
			name: "verifier is nil",
			args: args{verifier: nil},
			fields: fields{
				Keys: []*PublicKeyInfo{keyInfo},
			},
			wantErr: ErrVerifierIsNil,
		},
		{
			name: "system description record is nil",
			args: args{verifier: verifier},
			fields: fields{
				Keys: []*PublicKeyInfo{keyInfo},
			},
			wantErr: ErrSystemDescriptionIsNil,
		},
		{
			name: "keys are missing",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase: pubKey,
				Keys:      nil,
			},
			wantErr: ErrKeysAreMissing,
		},
		{
			name: "node pub key info is nil",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase: pubKey,
				Keys:      []*PublicKeyInfo{nil},
			},
			wantErr: ErrKeyIsNil,
		},

		{
			name: "pub key info identifier is empty",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase: pubKey,
				Keys:      []*PublicKeyInfo{{NodeIdentifier: "", PublicKey: pubKey}},
			},
			wantErr: ErrNodeIdentifierIsEmpty,
		},
		{
			name: "pub key is invalid",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase: pubKey,
				Keys:      []*PublicKeyInfo{{NodeIdentifier: "111", PublicKey: []byte{0, 0}}},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 2",
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
				Keys:      []*PublicKeyInfo{keyInfo},
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
				Keys:        []*PublicKeyInfo{keyInfo},
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
				Keys:                    tt.fields.Keys,
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
