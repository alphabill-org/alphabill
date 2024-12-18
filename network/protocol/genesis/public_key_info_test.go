package genesis

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
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
			args: args{[]*PublicKeyInfo{{
				NodeID:  "1",
				SignKey: []byte{1, 1},
				AuthKey: []byte{1, 2}}},
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
		NodeID  string
		SignKey []byte
		AuthKey []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name:    "missing node identifier",
			fields:  fields{"", pubKeyBytes, pubKeyBytes},
			wantErr: ErrPubKeyNodeIDIsEmpty.Error(),
		},
		{
			name:    "signing pub key is missing",
			fields:  fields{"1", nil, pubKeyBytes},
			wantErr: ErrPubKeyInfoSignKeyIsInvalid.Error(),
		},
		{
			name:    "signing pub key is invalid",
			fields:  fields{"1", []byte{1, 2}, pubKeyBytes},
			wantErr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name:    "enc pub key is missing",
			fields:  fields{"1", pubKeyBytes, nil},
			wantErr: ErrPubKeyInfoAuthKeyIsInvalid.Error(),
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
				NodeID:  tt.fields.NodeID,
				SignKey: tt.fields.SignKey,
				AuthKey: tt.fields.AuthKey,
			}
			require.ErrorContains(t, x.IsValid(), tt.wantErr)
		})
	}
}

func TestValidatorInfoUnique(t *testing.T) {
	_, signKey1Verifier := testsig.CreateSignerAndVerifier(t)
	signKey1, err := signKey1Verifier.MarshalPublicKey()
	require.NoError(t, err)
	_, authKey1Verifier := testsig.CreateSignerAndVerifier(t)
	authKey1, err := authKey1Verifier.MarshalPublicKey()
	require.NoError(t, err)
	_, signKey2Verifier := testsig.CreateSignerAndVerifier(t)
	signKey2, err := signKey2Verifier.MarshalPublicKey()
	require.NoError(t, err)
	_, authKey2Verifier := testsig.CreateSignerAndVerifier(t)
	authKey2, err := authKey2Verifier.MarshalPublicKey()
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
				{NodeID: "1", SignKey: []byte{1, 1}, AuthKey: []byte{1, 2}}},
			},
			wantErr: "pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "Duplicate node id",
			args: args{[]*PublicKeyInfo{
				{NodeID: "1", SignKey: signKey1, AuthKey: authKey1},
				{NodeID: "1", SignKey: signKey2, AuthKey: authKey2}},
			},
			wantErr: "duplicate node id:",
		},
		{
			name: "Duplicate sign key",
			args: args{[]*PublicKeyInfo{
				{NodeID: "1", SignKey: signKey1, AuthKey: authKey1},
				{NodeID: "2", SignKey: signKey1, AuthKey: authKey2}},
			},
			wantErr: "duplicate node signing key:",
		},
		{
			name: "Duplicate auth key",
			args: args{[]*PublicKeyInfo{
				{NodeID: "1", SignKey: signKey1, AuthKey: authKey1},
				{NodeID: "2", SignKey: signKey2, AuthKey: authKey1}},
			},
			wantErr: "duplicate node authentication key:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, ValidatorInfoUnique(tt.args.validators), tt.wantErr)
		})
	}
}

func TestPublicKeyInfo_NodeID(t *testing.T) {
	t.Run("Authentication key nil", func(t *testing.T) {
		x := &PublicKeyInfo{
			AuthKey: nil,
		}
		id, err := x.GetNodeID()
		require.ErrorContains(t, err, "authentication key marshal error: malformed public key: invalid length: 0")
		require.Empty(t, id)
	})
	t.Run("Authentication key empty", func(t *testing.T) {
		x := &PublicKeyInfo{
			AuthKey: make([]byte, 0),
		}
		id, err := x.GetNodeID()
		require.ErrorContains(t, err, "authentication key marshal error: malformed public key: invalid length: 0")
		require.Empty(t, id)
	})
	t.Run("Authentication key invalid", func(t *testing.T) {
		x := &PublicKeyInfo{
			AuthKey: []byte{1, 2, 3},
		}
		id, err := x.GetNodeID()
		require.ErrorContains(t, err, "authentication key marshal error: malformed public key: invalid length: 3")
		require.Empty(t, id)
	})
	t.Run("Authentication key invalid", func(t *testing.T) {
		_, verifier := testsig.CreateSignerAndVerifier(t)
		authKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		x := &PublicKeyInfo{
			AuthKey: authKey,
		}
		id, err := x.GetNodeID()
		require.NoError(t, err)
		require.NotEmpty(t, id)
	})
}
