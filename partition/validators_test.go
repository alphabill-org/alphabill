package partition

import (
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

var systemDescription = &types.PartitionDescriptionRecord{
	Version:             1,
	NetworkIdentifier:   5,
	PartitionIdentifier: 1,
	TypeIdLen:           8,
	UnitIdLen:           256,
	T2Timeout:           2500 * time.Millisecond,
}

func TestNewDefaultUnicityCertificateValidator_NotOk(t *testing.T) {
	_, v := testsig.CreateSignerAndVerifier(t)
	type args struct {
		systemDescription *types.PartitionDescriptionRecord
		rootTrustBase     types.RootTrustBase
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
				rootTrustBase:     trustbase.NewTrustBase(t, v),
				algorithm:         gocrypto.SHA256,
			},
			wantErr: types.ErrSystemDescriptionIsNil,
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
	rootTrust := trustbase.NewTrustBase(t, verifier)
	v, err := NewDefaultUnicityCertificateValidator(systemDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	require.ErrorIs(t, v.Validate(nil), types.ErrUnicityCertificateIsNil)
}

func TestDefaultUnicityCertificateValidator_ValidateOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	rootTrust := trustbase.NewTrustBase(t, verifier)
	v, err := NewDefaultUnicityCertificateValidator(systemDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	ir := &types.InputRecord{
		Version:      1,
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    make([]byte, 32),
		SummaryValue: make([]byte, 32),
		RoundNumber:  1,
		Timestamp:    types.NewTimestamp(),
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
		systemDescription *types.PartitionDescriptionRecord
		trustBase         types.RootTrustBase
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
				trustBase:         trustbase.NewTrustBase(t, v),
				algorithm:         gocrypto.SHA256,
			},
			wantErr: types.ErrSystemDescriptionIsNil,
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
	rootTrust := trustbase.NewTrustBase(t, verifier)
	v, err := NewDefaultBlockProposalValidator(systemDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	require.ErrorIs(t, v.Validate(nil, nil), blockproposal.ErrBlockProposalIsNil)
}

func TestDefaultNewDefaultBlockProposalValidator_ValidateOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	nodeSigner, nodeVerifier := testsig.CreateSignerAndVerifier(t)
	rootTrust := trustbase.NewTrustBase(t, verifier)
	v, err := NewDefaultBlockProposalValidator(systemDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	ir := &types.InputRecord{
		Version:      1,
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    make([]byte, 32),
		SummaryValue: make([]byte, 32),
		RoundNumber:  1,
		Timestamp:    types.NewTimestamp(),
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
		Partition:          uc.UnicityTreeCertificate.Partition,
		NodeIdentifier:     "1",
		UnicityCertificate: uc,
		Transactions: []*types.TransactionRecord{
			{
				TransactionOrder: testtransaction.NewTransactionOrderBytes(t),
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
		name                        string
		tx                          *types.TransactionOrder
		latestBlockNumber           uint64
		expectedPartitionIdentifier types.PartitionID
		errStr                      string
	}{
		{
			name:                        "tx is nil",
			tx:                          nil,
			latestBlockNumber:           10,
			expectedPartitionIdentifier: 0x01020304,
			errStr:                      "transaction is nil",
		},
		{
			name:                        "invalid partition identifier",
			tx:                          testtransaction.NewTransactionOrder(t), // default partitionID is 0x00000001
			latestBlockNumber:           10,
			expectedPartitionIdentifier: 0x01020304,
			errStr:                      "expected 01020304, got 00000001: invalid transaction partition identifier",
		},
		{
			name:                        "expired transaction",
			tx:                          testtransaction.NewTransactionOrder(t), // default timeout is 10
			latestBlockNumber:           11,
			expectedPartitionIdentifier: 0x00000001,
			errStr:                      "transaction timeout round is 10, current round is 11: transaction has timed out",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dtv := &DefaultTxValidator{
				partitionIdentifier: tt.expectedPartitionIdentifier,
			}
			err := dtv.Validate(tt.tx, tt.latestBlockNumber)
			require.ErrorContains(t, err, tt.errStr)
		})
	}
}
