package genesis

import (
	"testing"

	"github.com/alphabill-org/alphabill/common/crypto"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestNewValidatorTrustBase(t *testing.T) {
	type args struct {
		rootPublicInfo []*PublicKeyInfo
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]crypto.Verifier
		wantErr string
	}{
		{
			name:    "Validator info is nil",
			args:    args{nil},
			wantErr: ErrValidatorPublicInfoIsEmpty.Error(),
		},
		{
			name:    "From empty validator info",
			args:    args{[]*PublicKeyInfo{}},
			wantErr: ErrValidatorPublicInfoIsEmpty.Error(),
		},
		{
			name: "Invalid validator public key",
			args: args{[]*PublicKeyInfo{
				&PublicKeyInfo{
					NodeIdentifier:      "1",
					SigningPublicKey:    []byte{1, 1},
					EncryptionPublicKey: []byte{1, 2}}},
			},
			wantErr: "pubkey must be 33 bytes long, but is 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewValidatorTrustBase(tt.args.rootPublicInfo)
			require.ErrorContains(t, err, tt.wantErr)
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
		name    string
		fields  fields
		wantErr string
	}{
		{
			name:    "missing node identifier",
			fields:  fields{"", pubKeyBytes, pubKeyBytes},
			wantErr: ErrPubKeyNodeIdentifierIsEmpty.Error(),
		},
		{
			name:    "signing pub key is missing",
			fields:  fields{"1", nil, pubKeyBytes},
			wantErr: ErrPubKeyInfoSigningKeyIsInvalid.Error(),
		},
		{
			name:    "signing pub key is invalid",
			fields:  fields{"1", []byte{1, 2}, pubKeyBytes},
			wantErr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name:    "enc pub key is missing",
			fields:  fields{"1", pubKeyBytes, nil},
			wantErr: ErrPubKeyInfoEncryptionIsInvalid.Error(),
		},
		{
			name:    "enc pub key is invalid",
			fields:  fields{"1", pubKeyBytes, []byte{1}},
			wantErr: "pubkey must be 33 bytes long, but is 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PublicKeyInfo{
				NodeIdentifier:      tt.fields.NodeIdentifier,
				SigningPublicKey:    tt.fields.SigningPublicKey,
				EncryptionPublicKey: tt.fields.EncryptionPublicKey,
			}
			require.ErrorContains(t, x.IsValid(), tt.wantErr)
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
		name    string
		args    args
		wantErr string
	}{
		{
			name:    "Validator info is nil",
			args:    args{nil},
			wantErr: ErrValidatorPublicInfoIsEmpty.Error(),
		},
		{
			name:    "From empty validator info",
			args:    args{[]*PublicKeyInfo{}},
			wantErr: ErrValidatorPublicInfoIsEmpty.Error(),
		},
		{
			name: "Invalid validator public key",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: []byte{1, 1}, EncryptionPublicKey: []byte{1, 2}}},
			},
			wantErr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "Duplicate node id",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey1Bytes},
				{NodeIdentifier: "1", SigningPublicKey: signPubKey2Bytes, EncryptionPublicKey: encPubKey2Bytes}},
			},
			wantErr: "duplicated node id:",
		},
		{
			name: "Duplicate signing pub key",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey1Bytes},
				{NodeIdentifier: "2", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey2Bytes}},
			},
			wantErr: "duplicated node signing public key:",
		},
		{
			name: "Duplicate enc pub key",
			args: args{[]*PublicKeyInfo{
				{NodeIdentifier: "1", SigningPublicKey: signPubKey1Bytes, EncryptionPublicKey: encPubKey1Bytes},
				{NodeIdentifier: "2", SigningPublicKey: signPubKey2Bytes, EncryptionPublicKey: encPubKey1Bytes}},
			},
			wantErr: "duplicated node encryption public key:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, ValidatorInfoUnique(tt.args.validators), tt.wantErr)
		})
	}
}
