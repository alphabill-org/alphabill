package partition

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

var systemDescription = &genesis.SystemDescriptionRecord{
	SystemIdentifier: []byte{0, 0, 0, 0},
	T2Timeout:        2500,
}

func TestNewDefaultUnicityCertificateValidator_NotOk(t *testing.T) {
	_, v := testsig.CreateSignerAndVerifier(t)
	type args struct {
		systemDescription *genesis.SystemDescriptionRecord
		rootTrustBase     map[string]crypto.Verifier
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
				rootTrustBase:     map[string]crypto.Verifier{"test": v},
				algorithm:         gocrypto.SHA256,
			},
			wantErr: genesis.ErrSystemDescriptionIsNil,
		},
		{
			name: "trust base is nil",
			args: args{
				systemDescription: systemDescription,
				rootTrustBase:     nil,
				algorithm:         gocrypto.SHA256,
			},
			wantErr: certificates.ErrRootValidatorInfoMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDefaultUnicityCertificateValidator(tt.args.systemDescription, tt.args.rootTrustBase, tt.args.algorithm)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestDefaultUnicityCertificateValidator_ValidateNotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"test": verifier}
	v, err := NewDefaultUnicityCertificateValidator(systemDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	require.ErrorIs(t, v.Validate(nil), certificates.ErrUnicityCertificateIsNil)
}

func TestDefaultUnicityCertificateValidator_ValidateOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"test": verifier}
	v, err := NewDefaultUnicityCertificateValidator(systemDescription, rootTrust, gocrypto.SHA256)
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

func TestNewDefaultBlockProposalValidator_NotOk(t *testing.T) {
	_, v := testsig.CreateSignerAndVerifier(t)
	type args struct {
		systemDescription *genesis.SystemDescriptionRecord
		trustBase         map[string]crypto.Verifier
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
				trustBase:         map[string]crypto.Verifier{"test": v},
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
			wantErr: certificates.ErrRootValidatorInfoMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDefaultBlockProposalValidator(tt.args.systemDescription, tt.args.trustBase, tt.args.algorithm)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestDefaultNewDefaultBlockProposalValidator_ValidateNotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"test": verifier}
	v, err := NewDefaultBlockProposalValidator(systemDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	require.ErrorIs(t, v.Validate(nil, nil), blockproposal.ErrBlockProposalIsNil)
}

func TestDefaultNewDefaultBlockProposalValidator_ValidateOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	nodeSigner, nodeVerifier := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"test": verifier}
	v, err := NewDefaultBlockProposalValidator(systemDescription, rootTrust, gocrypto.SHA256)
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

	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     "1",
		UnicityCertificate: uc,
		Transactions:       []*txsystem.Transaction{testtransaction.RandomBillTransfer()},
	}
	err = bp.Sign(gocrypto.SHA256, nodeSigner)
	require.NoError(t, err)
	require.NoError(t, v.Validate(bp, nodeVerifier))
}
