package wallet

import (
	"io/ioutil"
	"path"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestTrustBase_Verify(t *testing.T) {
	tests := []struct {
		name       string
		validators []*genesis.PublicKeyInfo
		wantErrStr string
	}{
		{
			name:       "validators list is empty",
			validators: []*genesis.PublicKeyInfo{},
			wantErrStr: "validators list is empty",
		},
		{
			name: "missing trust base signing public key",
			validators: []*genesis.PublicKeyInfo{
				{
					NodeIdentifier:   "test",
					SigningPublicKey: nil,
				},
			},
			wantErrStr: "missing trust base signing public key",
		},
		{
			name: "missing trust base node identifier",
			validators: []*genesis.PublicKeyInfo{
				{
					NodeIdentifier:   "",
					SigningPublicKey: test.RandomBytes(32),
				},
			},
			wantErrStr: "missing trust base node identifier",
		},
		{
			name: "ok",
			validators: []*genesis.PublicKeyInfo{
				{
					NodeIdentifier:   "test",
					SigningPublicKey: test.RandomBytes(32),
				},
			},
			wantErrStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &TrustBase{
				RootValidators: tt.validators,
			}
			err := tb.Verify()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func TestReadTrustBaseFile(t *testing.T) {
	temp, err := ioutil.TempDir("", "test")
	require.NoError(t, err)

	pathToInvalidTrustBaseFile := path.Join(temp, "missing-validators-trust-base.json")
	require.NoError(t, util.WriteJsonFile(pathToInvalidTrustBaseFile, &TrustBase{nil}))

	pathToInvalidPubKeyFile := path.Join(temp, "invalid-pub-key-trust-base.json")
	require.NoError(t, util.WriteJsonFile(pathToInvalidPubKeyFile, &TrustBase{RootValidators: []*genesis.PublicKeyInfo{{
		NodeIdentifier:   "test",
		SigningPublicKey: []byte{0, 0, 0, 1},
	}}}))

	_, verifier := testsig.CreateSignerAndVerifier(t)
	pathToValidTrustBase := path.Join(temp, "trust-base.json")
	require.NoError(t, util.WriteJsonFile(pathToValidTrustBase, &TrustBase{RootValidators: []*genesis.PublicKeyInfo{{
		NodeIdentifier:   "test",
		SigningPublicKey: testsig.VerifierBytes(t, verifier),
	}}}))

	tests := []struct {
		name    string
		file    string
		want    map[string]crypto.Verifier
		wantErr string
	}{
		{
			name:    "trust base not found",
			file:    "trust-base-not-found.json",
			wantErr: "no such file or directory",
		},
		{
			name:    "trust base file verification fails",
			file:    pathToInvalidTrustBaseFile,
			wantErr: "validators list is empty",
		},
		{
			name:    "trust base contains invalid public key",
			file:    pathToInvalidPubKeyFile,
			wantErr: "pubkey must be 33 bytes long",
		},
		{
			name: "ok",
			file: pathToValidTrustBase,
			want: map[string]crypto.Verifier{
				"test": verifier,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadTrustBaseFile(tt.file)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadTrustBaseFile() got = %v, want %v", got, tt.want)
			}
		})
	}
}
