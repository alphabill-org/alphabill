package certificates

import (
	gocrypto "crypto"
	"strings"
	"testing"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

func TestUnicityCertificate_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign("test", signer)
	require.NoError(t, err)
	type fields struct {
		InputRecord            *InputRecord
		UnicityTreeCertificate *UnicityTreeCertificate
		UnicitySeal            *UnicitySeal
	}
	type args struct {
		verifiers             map[string]crypto.Verifier
		algorithm             gocrypto.Hash
		systemIdentifier      []byte
		systemDescriptionHash []byte
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		err    error
		errStr string
	}{
		{
			name: "invalid input record",
			fields: fields{
				InputRecord:            nil,
				UnicityTreeCertificate: uct,
				UnicitySeal:            seal,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			err: ErrInputRecordIsNil,
		},
		{
			name: "invalid uct",
			fields: fields{
				InputRecord:            ir,
				UnicityTreeCertificate: nil,
				UnicitySeal:            seal,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			err: ErrUnicityTreeCertificateIsNil,
		},
		{
			name: "invalid unicity seal",
			fields: fields{
				InputRecord:            ir,
				UnicityTreeCertificate: uct,
				UnicitySeal:            nil,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			err: ErrUnicitySealIsNil,
		},
		{
			name: "invalid root hash",
			fields: fields{
				InputRecord:            ir,
				UnicityTreeCertificate: uct,
				UnicitySeal:            seal,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			errStr: "does not match with the root hash of the unicity tree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &UnicityCertificate{
				InputRecord:            tt.fields.InputRecord,
				UnicityTreeCertificate: tt.fields.UnicityTreeCertificate,
				UnicitySeal:            tt.fields.UnicitySeal,
			}

			err := uc.IsValid(tt.args.verifiers, tt.args.algorithm, tt.args.systemIdentifier, tt.args.systemDescriptionHash)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.errStr))
			}
		})
	}
}

func TestUnicityCertificate_UCIsNil(t *testing.T) {
	var uc *UnicityCertificate
	require.ErrorIs(t, uc.IsValid(nil, gocrypto.SHA256, nil, nil), ErrUnicityCertificateIsNil)
}
