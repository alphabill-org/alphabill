package blockproposal

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

const dummyTimeStamp = 10000

var systemIdentifier = []byte{0, 0, 0, 1}

func TestBlockProposal_IsValid_NotOk(t *testing.T) {
	_, nodeVerifier := testsig.CreateSignerAndVerifier(t)
	_, trustBase := testsig.CreateSignerAndVerifier(t)
	type fields struct {
		SystemIdentifier   []byte
		NodeIdentifier     string
		UnicityCertificate *certificates.UnicityCertificate
		Transactions       []*txsystem.Transaction
	}
	type args struct {
		nodeSignatureVerifier crypto.Verifier
		ucTrustBase           map[string]crypto.Verifier
		algorithm             gocrypto.Hash
		systemIdentifier      []byte
		systemDescriptionHash []byte
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "node signature verifier is nil",
			fields: fields{
				SystemIdentifier: systemIdentifier,
				NodeIdentifier:   "1",
				Transactions:     []*txsystem.Transaction{},
			},
			args: args{
				nodeSignatureVerifier: nil,
				ucTrustBase:           map[string]crypto.Verifier{"1": trustBase},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      systemIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: ErrNodeVerifierIsNil,
		},
		{
			name: "uc trust base verifier is nil",
			fields: fields{
				SystemIdentifier: systemIdentifier,
				NodeIdentifier:   "1",
				Transactions:     []*txsystem.Transaction{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           nil,
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      systemIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: ErrTrustBaseIsNil,
		},
		{
			name: "invalid system identifier",
			fields: fields{
				SystemIdentifier: systemIdentifier,
				NodeIdentifier:   "1",
				Transactions:     []*txsystem.Transaction{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           map[string]crypto.Verifier{"1": trustBase},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      []byte{0, 0, 0, 2},
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: ErrInvalidSystemIdentifier,
		},
		{
			name: "block proposer id is missing",
			fields: fields{
				SystemIdentifier: systemIdentifier,
				Transactions:     []*txsystem.Transaction{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           map[string]crypto.Verifier{"1": trustBase},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      systemIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: errBlockProposerIDMissing,
		},
		{
			name: "uc is nil",
			fields: fields{
				SystemIdentifier:   systemIdentifier,
				NodeIdentifier:     "1",
				UnicityCertificate: nil,
				Transactions:       []*txsystem.Transaction{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           map[string]crypto.Verifier{"1": trustBase},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      systemIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: certificates.ErrUnicityCertificateIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &BlockProposal{
				SystemIdentifier:   tt.fields.SystemIdentifier,
				NodeIdentifier:     tt.fields.NodeIdentifier,
				UnicityCertificate: tt.fields.UnicityCertificate,
				Transactions:       tt.fields.Transactions,
			}
			err := bp.IsValid(tt.args.nodeSignatureVerifier, tt.args.ucTrustBase, tt.args.algorithm, tt.args.systemIdentifier, tt.args.systemDescriptionHash)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestBlockProposal_IsValid_BlockProposalIsNil(t *testing.T) {
	var bp *BlockProposal
	_, verifier := testsig.CreateSignerAndVerifier(t)
	ucTrustBase := map[string]crypto.Verifier{"1": verifier}
	err := bp.IsValid(verifier, ucTrustBase, gocrypto.SHA256, systemIdentifier, test.RandomBytes(32))
	require.ErrorIs(t, err, ErrBlockProposalIsNil)
}

func TestBlockProposal_SignAndVerify(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	sdrHash := test.RandomBytes(32)
	seal := &certificates.UnicitySeal{
		RootRoundInfo: &certificates.RootRoundInfo{
			RoundNumber:     1,
			Timestamp:       dummyTimeStamp,
			CurrentRootHash: test.RandomBytes(32),
		},
		CommitInfo: &certificates.CommitInfo{
			RootRoundInfoHash: test.RandomBytes(32),
			RootHash:          test.RandomBytes(32),
		},
		Signatures: map[string][]byte{"1": test.RandomBytes(32)},
	}
	bp := &BlockProposal{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   "1",
		UnicityCertificate: &certificates.UnicityCertificate{
			InputRecord: &certificates.InputRecord{
				PreviousHash: test.RandomBytes(32),
				Hash:         test.RandomBytes(32),
				BlockHash:    test.RandomBytes(32),
				SummaryValue: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      systemIdentifier,
				SiblingHashes:         [][]byte{test.RandomBytes(32)},
				SystemDescriptionHash: sdrHash,
			},
			UnicitySeal: seal,
		},
		Transactions: []*txsystem.Transaction{moneytesttx.RandomBillTransfer(t)},
	}
	err := bp.Sign(gocrypto.SHA256, signer)
	require.NoError(t, err)

	err = bp.Verify(gocrypto.SHA256, verifier)
	require.NoError(t, err)
}

func TestBlockProposal_InvalidSignature(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	sdrHash := test.RandomBytes(32)
	seal := &certificates.UnicitySeal{
		RootRoundInfo: &certificates.RootRoundInfo{
			RoundNumber:     1,
			Timestamp:       dummyTimeStamp,
			CurrentRootHash: test.RandomBytes(32),
		},
		CommitInfo: &certificates.CommitInfo{
			RootRoundInfoHash: test.RandomBytes(32),
			RootHash:          test.RandomBytes(32),
		},
		Signatures: map[string][]byte{"1": test.RandomBytes(32)},
	}
	bp := &BlockProposal{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   "1",
		UnicityCertificate: &certificates.UnicityCertificate{
			InputRecord: &certificates.InputRecord{
				PreviousHash: test.RandomBytes(32),
				Hash:         test.RandomBytes(32),
				BlockHash:    test.RandomBytes(32),
				SummaryValue: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      systemIdentifier,
				SiblingHashes:         [][]byte{test.RandomBytes(32)},
				SystemDescriptionHash: sdrHash,
			},
			UnicitySeal: seal,
		},
		Transactions: []*txsystem.Transaction{moneytesttx.RandomBillTransfer(t)},
	}
	err := bp.Sign(gocrypto.SHA256, signer)
	require.NoError(t, err)
	bp.Signature = test.RandomBytes(64)

	err = bp.Verify(gocrypto.SHA256, verifier)
	require.ErrorIs(t, err, errors.ErrVerificationFailed)
}
