package genesis

import (
	gocrypto "crypto"
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

func TestGenesisPartitionRecord_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)

	signerNode1, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signerNode2, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	type fields struct {
		Nodes                   []*PartitionNode
		Certificate             *certificates.UnicityCertificate
		SystemDescriptionRecord *SystemDescriptionRecord
	}
	type args struct {
		verifier      crypto.Verifier
		hashAlgorithm gocrypto.Hash
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErr    error
		wantErrStr string
	}{
		{
			name:    "verifier is nil",
			args:    args{verifier: nil},
			fields:  fields{},
			wantErr: ErrVerifierIsNil,
		},
		{
			name:    "nodes missing",
			args:    args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields:  fields{},
			wantErr: ErrNodesAreMissing,
		},
		{
			name: "system description record is nil",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes:                   []*PartitionNode{nil},
				SystemDescriptionRecord: nil,
			},
			wantErr: ErrSystemDescriptionIsNil,
		},
		{
			name: "contains nodes with same node identifier",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signerNode1),
					createPartitionNode(t, "1", signerNode2),
				},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 10},
			},
			wantErrStr: "duplicated node id: 1",
		},
		{
			name: "contains nodes with same public key",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signerNode1),
					createPartitionNode(t, "2", signerNode1),
				},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 10},
			},
			wantErrStr: "duplicated node public key",
		},
		{
			name: "certificate is nil",
			args: args{verifier: verifier, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signerNode1),
				},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 10},
			},
			wantErr: certificates.ErrUnicityCertificateIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisPartitionRecord{
				Nodes:                   tt.fields.Nodes,
				Certificate:             tt.fields.Certificate,
				SystemDescriptionRecord: tt.fields.SystemDescriptionRecord,
			}
			err := x.IsValid(tt.args.verifier, tt.args.hashAlgorithm)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.wantErrStr))
			}
		})
	}
}

func TestGenesisPartitionRecord_IsValid_Nil(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	var pr *GenesisPartitionRecord
	require.ErrorIs(t, ErrGenesisPartitionRecordIsNil, pr.IsValid(verifier, gocrypto.SHA256))
}

func createPartitionNode(t *testing.T, nodeID string, signer crypto.Signer) *PartitionNode {
	t.Helper()
	node1Verifier, err := signer.Verifier()
	require.NoError(t, err)
	node1VerifierPubKey, err := node1Verifier.MarshalPublicKey()
	require.NoError(t, err)
	p1Request := &p1.P1Request{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   nodeID,
		RootRoundNumber:  1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: make([]byte, 32),
		},
	}
	require.NoError(t, p1Request.Sign(signer))
	pr := &PartitionNode{
		NodeIdentifier: nodeID,
		PublicKey:      node1VerifierPubKey,
		P1Request:      p1Request,
	}
	return pr
}
