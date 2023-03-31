package genesis

import (
	gocrypto "crypto"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/certificates"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

func TestPartitionGenesis_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	verifiers := map[string]crypto.Verifier{"test": verifier}
	keyInfo := &PublicKeyInfo{
		NodeIdentifier:      "1",
		SigningPublicKey:    pubKey,
		EncryptionPublicKey: pubKey,
	}

	rootKeyInfo := &PublicKeyInfo{
		NodeIdentifier:      "1",
		SigningPublicKey:    pubKey,
		EncryptionPublicKey: pubKey,
	}

	type fields struct {
		SystemDescriptionRecord *SystemDescriptionRecord
		Certificate             *certificates.UnicityCertificate
		RootValidators          []*PublicKeyInfo
		Keys                    []*PublicKeyInfo
	}
	type args struct {
		verifier      map[string]crypto.Verifier
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
			wantErrStr: ErrVerifiersEmpty,
		},
		{
			name: "system description record is nil",
			args: args{verifier: verifiers},
			fields: fields{
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErr: ErrSystemDescriptionIsNil,
		},
		{
			name: "keys are missing",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           nil,
			},
			wantErr: ErrKeysAreMissing,
		},
		{
			name: "node signing key info is nil",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{nil},
			},
			wantErrStr: ErrValidatorPublicInfoIsEmpty.Error(),
		},

		{
			name: "key info identifier is empty",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys: []*PublicKeyInfo{
					{NodeIdentifier: "", SigningPublicKey: pubKey, EncryptionPublicKey: test.RandomBytes(33)},
				},
			},
			wantErrStr: "invalid partition node validator public key info",
		},
		{
			name: "signing pub key is invalid",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{{NodeIdentifier: "111", SigningPublicKey: []byte{0, 0}}},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "encryption pub key is invalid",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{{NodeIdentifier: "111", SigningPublicKey: pubKey, EncryptionPublicKey: []byte{0, 0}}},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "invalid root signing public key",
			args: args{verifier: verifiers},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "1", SigningPublicKey: []byte{0}, EncryptionPublicKey: pubKey}},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 1",
		},
		{
			name: "certificate is nil",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				Certificate:    nil,
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErr: ErrPartitionUnicityCertificateIsNil,
		},
		{
			name: "encryption key is nil",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "1", SigningPublicKey: pubKey, EncryptionPublicKey: nil}},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: "invalid root node public key info",
		},
		{
			name: "encryption key is invalid",
			args: args{verifier: verifiers, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				RootValidators: []*PublicKeyInfo{{NodeIdentifier: "1", SigningPublicKey: pubKey, EncryptionPublicKey: []byte{0, 0, 0, 0}}},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionGenesis{
				SystemDescriptionRecord: tt.fields.SystemDescriptionRecord,
				Certificate:             tt.fields.Certificate,
				RootValidators:          tt.fields.RootValidators,
				Keys:                    tt.fields.Keys,
			}
			err := x.IsValid(tt.args.verifier, tt.args.hashAlgorithm)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.ErrorContains(t, err, tt.wantErrStr)
			}
		})
	}
}

func TestPartitionGenesis_IsValid_Nil(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	var pr *PartitionGenesis
	verifiers := map[string]crypto.Verifier{"test": verifier}
	require.ErrorIs(t, pr.IsValid(verifiers, gocrypto.SHA256), ErrPartitionGenesisIsNil)
}
