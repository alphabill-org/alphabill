package partition

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
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
			wantErr: types.ErrRootValidatorInfoMissing,
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
	require.ErrorIs(t, v.Validate(nil), types.ErrUnicityCertificateIsNil)
}

func TestDefaultUnicityCertificateValidator_ValidateOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"test": verifier}
	v, err := NewDefaultUnicityCertificateValidator(systemDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	ir := &types.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    make([]byte, 32),
		SummaryValue: make([]byte, 32),
		RoundNumber:  1,
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
			wantErr: types.ErrRootValidatorInfoMissing,
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
	ir := &types.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    make([]byte, 32),
		SummaryValue: make([]byte, 32),
		RoundNumber:  1,
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
		Transactions: []*types.TransactionRecord{
			{
				TransactionOrder: testtransaction.NewTransaction(t),
				ServerMetadata: &types.ServerMetadata{
					ActualFee: 10,
				},
			},
		},
	}
	err = bp.Sign(gocrypto.SHA256, nodeSigner)
	require.NoError(t, err)
	require.NoError(t, v.Validate(bp, nodeVerifier))
}

func TestDefaultTxValidator_ValidateNotOk(t *testing.T) {
	tests := []struct {
		name                     string
		tx                       *types.TransactionOrder
		latestBlockNumber        uint64
		expectedSystemIdentifier []byte
		errStr                   string
	}{
		{
			name:                     "tx is nil",
			tx:                       nil,
			latestBlockNumber:        10,
			expectedSystemIdentifier: []byte{1, 2, 3, 4},
			errStr:                   "transaction is nil",
		},
		{
			name:                     "invalid system identifier",
			tx:                       testtransaction.NewTransaction(t), // default systemID is 0000
			latestBlockNumber:        10,
			expectedSystemIdentifier: []byte{1, 2, 3, 4},
			errStr:                   "expected 01020304, got 00000000: invalid transaction system identifier",
		},
		{
			name:                     "expired transaction",
			tx:                       testtransaction.NewTransaction(t), // default timeout is 10
			latestBlockNumber:        11,
			expectedSystemIdentifier: []byte{0, 0, 0, 0},
			errStr:                   "transaction has timed out: transaction timeout round is 10, current round is 11",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dtv := &DefaultTxValidator{
				systemIdentifier: tt.expectedSystemIdentifier,
			}
			err := dtv.Validate(tt.tx, tt.latestBlockNumber)
			require.ErrorContains(t, err, tt.errStr)
		})
	}
}
