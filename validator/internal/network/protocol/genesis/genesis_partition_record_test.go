package genesis

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/certification"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisPartitionRecord_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)

	signingKey1, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signingKey2, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, encryptionKey1 := testsig.CreateSignerAndVerifier(t)
	_, encryptionKey2 := testsig.CreateSignerAndVerifier(t)

	type fields struct {
		Nodes                   []*PartitionNode
		Certificate             *types.UnicityCertificate
		SystemDescriptionRecord *SystemDescriptionRecord
	}
	type args struct {
		verifier      map[string]crypto.Verifier
		hashAlgorithm gocrypto.Hash
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
			wantErrStr: ErrVerifiersEmpty.Error(),
		},
		{
			name:       "nodes missing",
			args:       args{verifier: map[string]crypto.Verifier{"test": verifier}, hashAlgorithm: gocrypto.SHA256},
			fields:     fields{},
			wantErrStr: ErrNodesAreMissing.Error(),
		},
		{
			name: "system description record is nil",
			args: args{verifier: map[string]crypto.Verifier{"test": verifier}, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes:                   []*PartitionNode{nil},
				SystemDescriptionRecord: nil,
			},
			wantErrStr: ErrSystemDescriptionIsNil.Error(),
		},
		{
			name: "contains nodes with same node identifier",
			args: args{verifier: map[string]crypto.Verifier{"test": verifier}, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
					createPartitionNode(t, "1", signingKey2, encryptionKey2),
				},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 10},
			},
			wantErrStr: "partition nodes validation failed, duplicated node id: 1",
		},
		{
			name: "contains nodes with same signing public key",
			args: args{verifier: map[string]crypto.Verifier{"test": verifier}, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
					createPartitionNode(t, "2", signingKey1, encryptionKey2),
				},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 10},
			},
			wantErrStr: "partition nodes validation failed, duplicated node signing public key",
		},
		{
			name: "contains nodes with same encryption public key",
			args: args{verifier: map[string]crypto.Verifier{"test": verifier}, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
					createPartitionNode(t, "2", signingKey2, encryptionKey1),
				},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 10},
			},
			wantErrStr: "partition nodes validation failed, duplicated node encryption public key",
		},
		{
			name: "certificate is nil",
			args: args{verifier: map[string]crypto.Verifier{"test": verifier}, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, encryptionKey1),
				},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 10},
			},
			wantErrStr: "unicity certificate validation failed, unicity certificate is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisPartitionRecord{
				Nodes:                   tt.fields.Nodes,
				Certificate:             tt.fields.Certificate,
				SystemDescriptionRecord: tt.fields.SystemDescriptionRecord,
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
	require.ErrorIs(t, ErrGenesisPartitionRecordIsNil, pr.IsValid(map[string]crypto.Verifier{"test": verifier}, gocrypto.SHA256))
}

func createPartitionNode(t *testing.T, nodeID string, signingKey crypto.Signer, encryptionPubKey crypto.Verifier) *PartitionNode {
	t.Helper()
	node1Verifier, err := signingKey.Verifier()
	require.NoError(t, err)
	node1VerifierPubKey, err := node1Verifier.MarshalPublicKey()
	require.NoError(t, err)

	encryptionPubKeyBytes, err := encryptionPubKey.MarshalPublicKey()
	require.NoError(t, err)

	request := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   nodeID,
		InputRecord: &types.InputRecord{
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
		NodeIdentifier:            nodeID,
		SigningPublicKey:          node1VerifierPubKey,
		EncryptionPublicKey:       encryptionPubKeyBytes,
		BlockCertificationRequest: request,
	}
	return pr
}
