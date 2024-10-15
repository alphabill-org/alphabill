package blockproposal

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/tree/imt"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/stretchr/testify/require"
)

const systemIdentifier types.SystemID = 1

func TestBlockProposal_IsValid_NotOk(t *testing.T) {
	_, nodeVerifier := testsig.CreateSignerAndVerifier(t)
	_, trustBase := testsig.CreateSignerAndVerifier(t)
	type fields struct {
		SystemIdentifier   types.SystemID
		NodeIdentifier     string
		UnicityCertificate *types.UnicityCertificate
		Transactions       []*types.TransactionRecord
	}
	type args struct {
		nodeSignatureVerifier crypto.Verifier
		ucTrustBase           types.RootTrustBase
		algorithm             gocrypto.Hash
		systemIdentifier      types.SystemID
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
				Transactions:     []*types.TransactionRecord{Version: 1},
			},
			args: args{
				nodeSignatureVerifier: nil,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
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
				Transactions:     []*types.TransactionRecord{Version: 1},
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
				Transactions:     []*types.TransactionRecord{Version: 1},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      2,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: ErrInvalidSystemIdentifier,
		},
		{
			name: "block proposer id is missing",
			fields: fields{
				SystemIdentifier: systemIdentifier,
				Transactions:     []*types.TransactionRecord{Version: 1},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
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
				Transactions:       []*types.TransactionRecord{Version: 1},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      systemIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: types.ErrUnicityCertificateIsNil,
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
	ucTrustBase := trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": verifier})
	err := bp.IsValid(verifier, ucTrustBase, gocrypto.SHA256, systemIdentifier, test.RandomBytes(32))
	require.ErrorIs(t, err, ErrBlockProposalIsNil)
}

func TestBlockProposal_SignAndVerify(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	sdrHash := test.RandomBytes(32)
	seal := &types.UnicitySeal{Version: 1,
		RootChainRoundNumber: 1,
		Timestamp:            10000,
		PreviousHash:         test.RandomBytes(32),
		Hash:                 test.RandomBytes(32),
		Signatures:           map[string][]byte{"1": test.RandomBytes(32)},
	}
	bp := &BlockProposal{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   "1",
		UnicityCertificate: &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1,
				PreviousHash: test.RandomBytes(32),
				Hash:         test.RandomBytes(32),
				BlockHash:    test.RandomBytes(32),
				SummaryValue: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{Version: 1,
				SystemIdentifier:         systemIdentifier,
				HashSteps:                []*imt.PathItem{{Key: test.RandomBytes(4), Hash: test.RandomBytes(32)}},
				PartitionDescriptionHash: sdrHash,
			},
			UnicitySeal: seal,
		},
		Transactions: []*types.TransactionRecord{Version: 1, {
			TransactionOrder: &types.TransactionOrder{
				Payload: types.Payload{
					SystemID:       0,
					Type:           22,
					UnitID:         nil,
					Attributes:     nil,
					ClientMetadata: nil,
				},
				AuthProof: nil,
				FeeProof:  nil,
			},
			ServerMetadata: &types.ServerMetadata{
				ActualFee: 10,
			},
		}},
	}
	err := bp.Sign(gocrypto.SHA256, signer)
	require.NoError(t, err)

	err = bp.Verify(gocrypto.SHA256, verifier)
	require.NoError(t, err)
}

func TestBlockProposal_InvalidSignature(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	sdrHash := test.RandomBytes(32)
	seal := &types.UnicitySeal{Version: 1,
		RootChainRoundNumber: 1,
		PreviousHash:         test.RandomBytes(32),
		Hash:                 test.RandomBytes(32),
		Timestamp:            10000,
		Signatures:           map[string][]byte{"1": test.RandomBytes(32)},
	}
	bp := &BlockProposal{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   "1",
		UnicityCertificate: &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1,
				PreviousHash: test.RandomBytes(32),
				Hash:         test.RandomBytes(32),
				BlockHash:    test.RandomBytes(32),
				SummaryValue: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{Version: 1,
				SystemIdentifier:         systemIdentifier,
				HashSteps:                []*imt.PathItem{{Key: test.RandomBytes(4), Hash: test.RandomBytes(32)}},
				PartitionDescriptionHash: sdrHash,
			},
			UnicitySeal: seal,
		},
		Transactions: []*types.TransactionRecord{Version: 1, {TransactionOrder: &types.TransactionOrder{
			Payload: types.Payload{
				SystemID:       0,
				Type:           22,
				UnitID:         nil,
				Attributes:     nil,
				ClientMetadata: nil,
			},
			AuthProof: nil,
			FeeProof:  nil,
		},
			ServerMetadata: &types.ServerMetadata{
				ActualFee: 10,
			}}},
	}
	err := bp.Sign(gocrypto.SHA256, signer)
	require.NoError(t, err)
	bp.Signature = test.RandomBytes(64)

	err = bp.Verify(gocrypto.SHA256, verifier)
	require.ErrorContains(t, err, "verification failed")
}
