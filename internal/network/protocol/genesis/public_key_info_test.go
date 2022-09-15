package genesis

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNewValidatorTrustBase(t *testing.T) {
	type args struct {
		rootPublicInfo []*PublicKeyInfo
	}
	tests := []struct {
		name       string
		args       args
		want       map[string]crypto.Verifier
		wantErr    error
		wantErrStr string
	}{
		{
			name:    "Validator info is nil",
			args:    args{nil},
			wantErr: ErrValidatorPublicInfoIsEmpty,
		},
		{
			name:    "From empty validator info",
			args:    args{[]*PublicKeyInfo{}},
			wantErr: ErrValidatorPublicInfoIsEmpty,
		},
		{
			name: "Invalid validator public key",
			args: args{[]*PublicKeyInfo{
				&PublicKeyInfo{
					NodeIdentifier:      "1",
					SigningPublicKey:    []byte{1, 1},
					EncryptionPublicKey: []byte{1, 2}}},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewValidatorTrustBase(tt.args.rootPublicInfo)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.wantErrStr))
			}

		})
	}
}

func TestPublicKeyInfo_IsValid(t *testing.T) {
	_, pubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := pubKey.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
		NodeIdentifier      string
		SigningPublicKey    []byte
		EncryptionPublicKey []byte
	}
	tests := []struct {
		name       string
		fields     fields
		wantErr    error
		wantErrStr string
	}{
		{
			name:    "missing node identifier",
			fields:  fields{"", pubKeyBytes, pubKeyBytes},
			wantErr: ErrNodeIdentifierIsEmpty,
		},
		{
			name:    "signing pub key is missing",
			fields:  fields{"1", nil, pubKeyBytes},
			wantErr: ErrSigningPublicKeyIsInvalid,
		},
		{
			name:       "signing pub key is invalid",
			fields:     fields{"1", []byte{1, 2}, pubKeyBytes},
			wantErrStr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name:    "enc pub key is missing",
			fields:  fields{"1", pubKeyBytes, nil},
			wantErr: ErrEncryptionPublicKeyIsInvalid,
		},
		{
			name:       "enc pub key is invalid",
			fields:     fields{"1", pubKeyBytes, []byte{1}},
			wantErrStr: "pubkey must be 33 bytes long, but is 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PublicKeyInfo{
				NodeIdentifier:      tt.fields.NodeIdentifier,
				SigningPublicKey:    tt.fields.SigningPublicKey,
				EncryptionPublicKey: tt.fields.EncryptionPublicKey,
			}
			err := x.IsValid()
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.wantErrStr))
			}
		})
	}
}

func TestValidatorInfoUnique(t *testing.T) {
	_, signPubKey1 := testsig.CreateSignerAndVerifier(t)
	signPubKey1Bytes, err := signPubKey1.MarshalPublicKey()
	require.NoError(t, err)
	_, encPubKey1 := testsig.CreateSignerAndVerifier(t)
	encPubKey1Bytes, err := encPubKey1.MarshalPublicKey()
	require.NoError(t, err)
	_, signPubKey2 := testsig.CreateSignerAndVerifier(t)
	signPubKey2Bytes, err := signPubKey2.MarshalPublicKey()
	require.NoError(t, err)
	_, encPubKey2 := testsig.CreateSignerAndVerifier(t)
	encPubKey2Bytes, err := encPubKey2.MarshalPublicKey()
	require.NoError(t, err)
	type args struct {
		validators []*PublicKeyInfo
	}
	tests := []struct {
		name       string
		args       args
		wantErr    error
		wantErrStr string
	}{
		{
			name:    "Validator info is nil",
			args:    args{nil},
			wantErr: ErrValidatorPublicInfoIsEmpty,
		},
		{
			name:    "From empty validator info",
			args:    args{[]*PublicKeyInfo{}},
			wantErr: ErrValidatorPublicInfoIsEmpty,
		},
		{
			name: "Invalid validator public key",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: []byte{1, 1}, EncryptionPublicKey: []byte{1, 2}}},
			},
			wantErrStr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "Duplicate node id",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey1Bytes},
				{NodeIdentifier: "1", SigningPublicKey: signPubKey2Bytes, EncryptionPublicKey: encPubKey2Bytes}},
			},
			wantErrStr: "duplicated node id:",
		},
		{
			name: "Duplicate signing pub key",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey1Bytes},
				{NodeIdentifier: "2", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey2Bytes}},
			},
			wantErrStr: "duplicated node signing public key:",
		},
		{
			name: "Duplicate enc pub key",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey1Bytes},
				{NodeIdentifier: "2", SigningPublicKey: signPubKey2Bytes, EncryptionPublicKey: encPubKey1Bytes}},
			},
			wantErrStr: "duplicated node encryption public key:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatorInfoUnique(tt.args.validators)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.wantErrStr))
			}
		})
	}
}
