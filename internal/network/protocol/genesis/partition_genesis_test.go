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

	keyInfo := &PublicKeyInfo{
		NodeIdentifier:      "1",
		SigningPublicKey:    pubKey,
		EncryptionPublicKey: pubKey,
	}

	type fields struct {
		SystemDescriptionRecord *SystemDescriptionRecord
		Certificate             *certificates.UnicityCertificate
		Keys                    []*PublicKeyInfo
		TrustBase               []byte
		EncryptionKey           []byte
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
				Keys:          []*PublicKeyInfo{keyInfo},
				EncryptionKey: pubKey,
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
			name: "node signing key info is nil",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase:     pubKey,
				EncryptionKey: pubKey,
				Keys:          []*PublicKeyInfo{nil},
			},
			wantErr: ErrKeyIsNil,
		},

		{
			name: "key info identifier is empty",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase:     pubKey,
				EncryptionKey: pubKey,
				Keys: []*PublicKeyInfo{
					{NodeIdentifier: "", SigningPublicKey: pubKey, EncryptionPublicKey: test.RandomBytes(33)},
				},
			},
			wantErr: ErrNodeIdentifierIsEmpty,
		},
		{
			name: "signing pub key is invalid",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase:     pubKey,
				EncryptionKey: pubKey,
				Keys:          []*PublicKeyInfo{{NodeIdentifier: "111", SigningPublicKey: []byte{0, 0}}},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "encryption pub key is invalid",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase:     pubKey,
				EncryptionKey: pubKey,
				Keys:          []*PublicKeyInfo{{NodeIdentifier: "111", SigningPublicKey: pubKey, EncryptionPublicKey: []byte{0, 0}}},
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
				TrustBase:     []byte{0},
				EncryptionKey: pubKey,
				Keys:          []*PublicKeyInfo{keyInfo},
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
				TrustBase:     pubKey,
				Certificate:   nil,
				EncryptionKey: pubKey,
				Keys:          []*PublicKeyInfo{keyInfo},
			},
			wantErr: certificates.ErrUnicityCertificateIsNil,
		},
		{
			name: "encryption key is nil",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase:     pubKey,
				EncryptionKey: nil,
				Keys:          []*PublicKeyInfo{keyInfo},
			},
			wantErr: ErrRootChainEncryptionKeyMissing,
		},
		{
			name: "encryption key is invalid",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 0},
					T2Timeout:        100,
				},
				TrustBase:     pubKey,
				EncryptionKey: []byte{0, 0, 0, 0},
				Keys:          []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionGenesis{
				SystemDescriptionRecord: tt.fields.SystemDescriptionRecord,
				Certificate:             tt.fields.Certificate,
				TrustBase:               tt.fields.TrustBase,
				EncryptionKey:           tt.fields.EncryptionKey,
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
	require.ErrorIs(t, pr.IsValid(verifier, gocrypto.SHA256), ErrPartitionGenesisIsNil)
}
