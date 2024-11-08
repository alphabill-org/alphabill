package genesis

import (
	"crypto"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/stretchr/testify/require"
)

func TestGenesisPartitionRecord_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)

	signingKey1, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signingKey2, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, encryptionKey1 := testsig.CreateSignerAndVerifier(t)
	_, encryptionKey2 := testsig.CreateSignerAndVerifier(t)
	validPDR := &types.PartitionDescriptionRecord{
		Version:             1,
		NetworkIdentifier:   5,
		PartitionIdentifier: 1,
		TypeIdLen:           8,
		UnitIdLen:           256,
		T2Timeout:           1 * time.Second,
	}

	type fields struct {
		Nodes                []*PartitionNode
		Certificate          *types.UnicityCertificate
		PartitionDescription *types.PartitionDescriptionRecord
	}
	type args struct {
		verifier      types.RootTrustBase
		hashAlgorithm crypto.Hash
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name:       "verifier is nil",
			args:       args{verifier: nil},
			fields:     fields{},
			wantErrStr: ErrTrustBaseIsNil.Error(),
		},
		{
			name:       "nodes missing",
			args:       args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields:     fields{},
			wantErrStr: ErrNodesAreMissing.Error(),
		},
		{
			name: "system description record is nil",
			args: args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes:                []*PartitionNode{nil},
				PartitionDescription: nil,
			},
			wantErrStr: types.ErrSystemDescriptionIsNil.Error(),
		},
		{
			name: "contains nodes with same node identifier",
			args: args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
					createPartitionNode(t, "1", signingKey2, encryptionKey2),
				},
				PartitionDescription: validPDR,
			},
			wantErrStr: "invalid partition nodes: duplicated node id: 1",
		},
		{
			name: "contains nodes with same signing public key",
			args: args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
					createPartitionNode(t, "2", signingKey1, encryptionKey2),
				},
				PartitionDescription: validPDR,
			},
			wantErrStr: "invalid partition nodes: duplicated node signing public key",
		},
		{
			name: "contains nodes with same encryption public key",
			args: args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
					createPartitionNode(t, "2", signingKey2, encryptionKey1),
				},
				PartitionDescription: validPDR,
			},
			wantErrStr: "invalid partition nodes: duplicated node encryption public key",
		},
		{
			name: "certificate is nil",
			args: args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
				},
				PartitionDescription: validPDR,
			},
			wantErrStr: "invalid unicity certificate: invalid unicity certificate: unicity certificate is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisPartitionRecord{
				Version:              1,
				Nodes:                tt.fields.Nodes,
				Certificate:          tt.fields.Certificate,
				PartitionDescription: tt.fields.PartitionDescription,
			}
			err = x.IsValid(tt.args.verifier, tt.args.hashAlgorithm)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				// must not be error then
				require.NoError(t, err)
			}
		})
	}
}

func TestGenesisPartitionRecord_IsValid_Nil(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	var pr *GenesisPartitionRecord
	require.ErrorIs(t, pr.IsValid(testtb.NewTrustBase(t, verifier), crypto.SHA256), ErrGenesisPartitionRecordIsNil)
}

func createPartitionNode(t *testing.T, nodeID string, signingKey abcrypto.Signer, encryptionPubKey abcrypto.Verifier) *PartitionNode {
	t.Helper()
	node1Verifier, err := signingKey.Verifier()
	require.NoError(t, err)
	node1VerifierPubKey, err := node1Verifier.MarshalPublicKey()
	require.NoError(t, err)

	encryptionPubKeyBytes, err := encryptionPubKey.MarshalPublicKey()
	require.NoError(t, err)

	request := &certification.BlockCertificationRequest{
		Partition:      1,
		NodeIdentifier: nodeID,
		InputRecord: &types.InputRecord{
			Version:      1,
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: make([]byte, 32),
			RoundNumber:  1,
		},
		RootRoundNumber: 1,
	}
	require.NoError(t, request.Sign(signingKey))
	pr := &PartitionNode{
		Version:                    1,
		NodeIdentifier:             nodeID,
		SigningPublicKey:           node1VerifierPubKey,
		EncryptionPublicKey:        encryptionPubKeyBytes,
		BlockCertificationRequest:  request,
		PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1},
	}
	return pr
}
