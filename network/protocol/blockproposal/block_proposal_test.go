package blockproposal

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/tree/imt"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testcerts "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/stretchr/testify/require"
)

const partitionIdentifier types.PartitionID = 1

func TestBlockProposal_IsValid_NotOk(t *testing.T) {
	_, nodeVerifier := testsig.CreateSignerAndVerifier(t)
	ucSigner, trustBase := testsig.CreateSignerAndVerifier(t)
	type fields struct {
		PartitionIdentifier types.PartitionID
		NodeIdentifier      string
		UnicityCertificate  *types.UnicityCertificate
		TechnicalRecord     certification.TechnicalRecord
		Transactions        []*types.TransactionRecord
	}
	type args struct {
		nodeSignatureVerifier crypto.Verifier
		ucTrustBase           types.RootTrustBase
		algorithm             gocrypto.Hash
		partitionIdentifier   types.PartitionID
		systemDescriptionHash []byte
	}

	pdr := &types.PartitionDescriptionRecord{
		PartitionIdentifier: partitionIdentifier,
	}
	tr := certification.TechnicalRecord{
		Round:    1,
		Epoch:    1,
		Leader:   "anyone",
		StatHash: []byte{0},
		FeeHash:  []byte{0},
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name: "node signature verifier is nil",
			fields: fields{
				PartitionIdentifier: partitionIdentifier,
				NodeIdentifier:      "1",
				Transactions:        []*types.TransactionRecord{},
			},
			args: args{
				nodeSignatureVerifier: nil,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
				algorithm:             gocrypto.SHA256,
				partitionIdentifier:   partitionIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: ErrNodeVerifierIsNil.Error(),
		},
		{
			name: "uc trust base verifier is nil",
			fields: fields{
				PartitionIdentifier: partitionIdentifier,
				NodeIdentifier:      "1",
				Transactions:        []*types.TransactionRecord{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           nil,
				algorithm:             gocrypto.SHA256,
				partitionIdentifier:   partitionIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: ErrTrustBaseIsNil.Error(),
		},
		{
			name: "invalid partition identifier",
			fields: fields{
				PartitionIdentifier: partitionIdentifier,
				NodeIdentifier:      "1",
				Transactions:        []*types.TransactionRecord{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
				algorithm:             gocrypto.SHA256,
				partitionIdentifier:   2,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: ErrInvalidPartitionIdentifier.Error(),
		},
		{
			name: "block proposer id is missing",
			fields: fields{
				PartitionIdentifier: partitionIdentifier,
				Transactions:        []*types.TransactionRecord{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
				algorithm:             gocrypto.SHA256,
				partitionIdentifier:   partitionIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: errBlockProposerIDMissing.Error(),
		},
		{
			name: "uc is nil",
			fields: fields{
				PartitionIdentifier: partitionIdentifier,
				NodeIdentifier:      "1",
				UnicityCertificate:  nil,
				Transactions:        []*types.TransactionRecord{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": trustBase}),
				algorithm:             gocrypto.SHA256,
				partitionIdentifier:   partitionIdentifier,
				systemDescriptionHash: test.RandomBytes(32),
			},
			wantErr: types.ErrUnicityCertificateIsNil.Error(),
		},
		{
			name: "tr hash mismatch",
			fields: fields{
				PartitionIdentifier: partitionIdentifier,
				NodeIdentifier:      "1",
				UnicityCertificate: testcerts.CreateUnicityCertificate(
					t, ucSigner, &types.InputRecord{
						Version:      1,
						PreviousHash: test.RandomBytes(32),
						Hash:         test.RandomBytes(32),
						BlockHash:    test.RandomBytes(32),
						SummaryValue: test.RandomBytes(32),
					}, pdr, 1, []byte{0}, make([]byte, 32)),
				TechnicalRecord:     tr,
				Transactions:        []*types.TransactionRecord{},
			},
			args: args{
				nodeSignatureVerifier: nodeVerifier,
				ucTrustBase:           trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"test": trustBase}),
				algorithm:             gocrypto.SHA256,
				partitionIdentifier:   partitionIdentifier,
				systemDescriptionHash: pdr.Hash(gocrypto.SHA256),
			},
			wantErr: "hash mismatch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &BlockProposal{
				Partition:          tt.fields.PartitionIdentifier,
				NodeIdentifier:     tt.fields.NodeIdentifier,
				UnicityCertificate: tt.fields.UnicityCertificate,
				Technical:          tt.fields.TechnicalRecord,
				Transactions:       tt.fields.Transactions,
			}
			err := bp.IsValid(tt.args.nodeSignatureVerifier, tt.args.ucTrustBase, tt.args.algorithm, tt.args.partitionIdentifier, tt.args.systemDescriptionHash)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestBlockProposal_IsValid_BlockProposalIsNil(t *testing.T) {
	var bp *BlockProposal
	_, verifier := testsig.CreateSignerAndVerifier(t)
	ucTrustBase := trustbase.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": verifier})
	err := bp.IsValid(verifier, ucTrustBase, gocrypto.SHA256, partitionIdentifier, test.RandomBytes(32))
	require.ErrorIs(t, err, ErrBlockProposalIsNil)
}

func TestBlockProposal_SignAndVerify(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	sdrHash := test.RandomBytes(32)
	seal := &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: 1,
		Timestamp:            10000,
		PreviousHash:         test.RandomBytes(32),
		Hash:                 test.RandomBytes(32),
		Signatures:           map[string]hex.Bytes{"1": test.RandomBytes(32)},
	}
	tx, err := (&types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			PartitionID:    0,
			Type:           22,
			UnitID:         nil,
			Attributes:     nil,
			ClientMetadata: nil,
		},
		AuthProof: nil,
		FeeProof:  nil,
	}).MarshalCBOR()
	require.NoError(t, err)
	bp := &BlockProposal{
		Partition:      partitionIdentifier,
		NodeIdentifier: "1",
		UnicityCertificate: &types.UnicityCertificate{
			Version: 1,
			InputRecord: &types.InputRecord{
				Version:      1,
				PreviousHash: test.RandomBytes(32),
				Hash:         test.RandomBytes(32),
				BlockHash:    test.RandomBytes(32),
				SummaryValue: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				Version:   1,
				Partition: partitionIdentifier,
				HashSteps: []*imt.PathItem{{Key: test.RandomBytes(4), Hash: test.RandomBytes(32)}},
				PDRHash:   sdrHash,
			},
			UnicitySeal: seal,
		},
		Transactions: []*types.TransactionRecord{{
			TransactionOrder: tx,
			ServerMetadata: &types.ServerMetadata{
				ActualFee: 10,
			},
		}},
	}
	require.NoError(t, bp.Sign(gocrypto.SHA256, signer))

	require.NoError(t, bp.Verify(gocrypto.SHA256, verifier))
}

func TestBlockProposal_InvalidSignature(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	sdrHash := test.RandomBytes(32)
	seal := &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: 1,
		PreviousHash:         test.RandomBytes(32),
		Hash:                 test.RandomBytes(32),
		Timestamp:            10000,
		Signatures:           map[string]hex.Bytes{"1": test.RandomBytes(32)},
	}
	tx, err := (&types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			PartitionID:    0,
			Type:           22,
			UnitID:         nil,
			Attributes:     nil,
			ClientMetadata: nil,
		},
		AuthProof: nil,
		FeeProof:  nil,
	}).MarshalCBOR()
	require.NoError(t, err)
	bp := &BlockProposal{
		Partition:      partitionIdentifier,
		NodeIdentifier: "1",
		UnicityCertificate: &types.UnicityCertificate{
			Version: 1,
			InputRecord: &types.InputRecord{
				Version:      1,
				PreviousHash: test.RandomBytes(32),
				Hash:         test.RandomBytes(32),
				BlockHash:    test.RandomBytes(32),
				SummaryValue: test.RandomBytes(32),
			},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				Version:   1,
				Partition: partitionIdentifier,
				HashSteps: []*imt.PathItem{{Key: test.RandomBytes(4), Hash: test.RandomBytes(32)}},
				PDRHash:   sdrHash,
			},
			UnicitySeal: seal,
		},
		Transactions: []*types.TransactionRecord{{
			TransactionOrder: tx,
			ServerMetadata: &types.ServerMetadata{
				ActualFee: 10,
			}}},
	}
	err = bp.Sign(gocrypto.SHA256, signer)
	require.NoError(t, err)
	bp.Signature = test.RandomBytes(64)

	err = bp.Verify(gocrypto.SHA256, verifier)
	require.ErrorContains(t, err, "verification failed")
}
