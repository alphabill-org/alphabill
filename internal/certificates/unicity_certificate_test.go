package certificates

import (
	gocrypto "crypto"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

func TestUnicityCertificate_IsValid(t *testing.T) {
	signer, verifier := generateSigner(t)
	seal := &UnicitySeal{
		RootChainBlockNumber: 1,
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign(signer)
	require.NoError(t, err)
	type fields struct {
		InputRecord            *InputRecord
		UnicityTreeCertificate *UnicityTreeCertificate
		UnicitySeal            *UnicitySeal
	}
	type args struct {
		verifier              crypto.Verifier
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
				verifier:              verifier,
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
				verifier:              verifier,
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
				verifier:              verifier,
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
				verifier:              verifier,
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

			err := uc.IsValid(tt.args.verifier, tt.args.algorithm, tt.args.systemIdentifier, tt.args.systemDescriptionHash)
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
