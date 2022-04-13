package partition

import (
	gocrypto "crypto"
	"testing"

	testcertificates "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

var systemDescription = &genesis.SystemDescriptionRecord{
	SystemIdentifier: []byte{0, 0, 0, 0},
	T2Timeout:        2500,
}

func TestNewDefaultUnicityCertificateValidator_NotOk(t *testing.T) {
	_, v := testsig.CreateSignerAndVerifier(t)
	type args struct {
		systemDescription *genesis.SystemDescriptionRecord
		trustBase         crypto.Verifier
		algorithm         gocrypto.Hash
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "system description record is nil",
			args: args{
				systemDescription: nil,
				trustBase:         v,
				algorithm:         gocrypto.SHA256,
			},
			wantErr: genesis.ErrSystemDescriptionIsNil,
		},
		{
			name: "trust base is nil",
			args: args{
				systemDescription: systemDescription,
				trustBase:         nil,
				algorithm:         gocrypto.SHA256,
			},
			wantErr: certificates.ErrVerifierIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDefaultUnicityCertificateValidator(tt.args.systemDescription, tt.args.trustBase, tt.args.algorithm)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestDefaultUnicityCertificateValidator_ValidateNotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	v, err := NewDefaultUnicityCertificateValidator(systemDescription, verifier, gocrypto.SHA256)
	require.NoError(t, err)
	require.ErrorIs(t, v.Validate(nil), certificates.ErrUnicityCertificateIsNil)
}

func TestDefaultUnicityCertificateValidator_ValidateOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	v, err := NewDefaultUnicityCertificateValidator(systemDescription, verifier, gocrypto.SHA256)
	require.NoError(t, err)
	ir := &certificates.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    make([]byte, 32),
		SummaryValue: make([]byte, 32),
	}
	uc := testcertificates.CreateUnicityCertificate(
		t,
		signer,
		ir,
		systemDescription,
		1,
		make([]byte, 32),
	)
	require.NoError(t, v.Validate(uc))
}