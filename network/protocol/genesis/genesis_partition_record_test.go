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

var validPDR = &types.PartitionDescriptionRecord{
	Version:     1,
	NetworkID:   5,
	PartitionID: 1,
	TypeIDLen:   8,
	UnitIDLen:   256,
	T2Timeout:   1 * time.Second,
}

func TestGenesisPartitionRecord_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)

	signingKey1, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signingKey2, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, authVerifier1 := testsig.CreateSignerAndVerifier(t)
	_, authVerifier2 := testsig.CreateSignerAndVerifier(t)

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
			name:       "nodes missing",
			args:       args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields:     fields{},
			wantErrStr: ErrNodesAreMissing.Error(),
		},
		{
			name: "partition description record is nil",
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
					createPartitionNode(t, "1", signingKey1, authVerifier1),
					createPartitionNode(t, "1", signingKey2, authVerifier2),
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
					createPartitionNode(t, "1", signingKey1, authVerifier1),
					createPartitionNode(t, "2", signingKey1, authVerifier2),
				},
				PartitionDescription: validPDR,
			},
			wantErrStr: "invalid partition nodes: duplicated node signing key",
		},
		{
			name: "contains nodes with same authentication key",
			args: args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, authVerifier1),
					createPartitionNode(t, "2", signingKey2, authVerifier1),
				},
				PartitionDescription: validPDR,
			},
			wantErrStr: "invalid partition nodes: duplicated node authentication key",
		},
		{
			name: "contains nodes with wrong partition identifier",
			args: args{verifier: nil, hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, authVerifier1),
				},
				PartitionDescription: &types.PartitionDescriptionRecord{
					Version:     1,
					NetworkID:   5,
					PartitionID: 2,
					TypeIDLen:   8,
					UnitIDLen:   256,
					T2Timeout:   time.Second,
				},
			},
			wantErrStr: "partition id 00000002 node 1 invalid blockCertificationRequest partition id 00000001",
		},
		{
			name: "certificate is nil",
			args: args{verifier: testtb.NewTrustBase(t, verifier), hashAlgorithm: crypto.SHA256},
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1, authVerifier1),
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
				Validators:           tt.fields.Nodes,
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

func createPartitionNode(t *testing.T, nodeID string, signer abcrypto.Signer, authVerifier abcrypto.Verifier) *PartitionNode {
	t.Helper()
	signVerifier, err := signer.Verifier()
	require.NoError(t, err)
	signKey, err := signVerifier.MarshalPublicKey()
	require.NoError(t, err)

	authKey, err := authVerifier.MarshalPublicKey()
	require.NoError(t, err)

	request := &certification.BlockCertificationRequest{
		PartitionID: 1,
		NodeID:      nodeID,
		InputRecord: &types.InputRecord{
			Version:      1,
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: make([]byte, 32),
			RoundNumber:  1,
			Timestamp:    types.NewTimestamp(),
		},
	}
	require.NoError(t, request.Sign(signer))
	pr := &PartitionNode{
		Version:                    1,
		NodeID:                     nodeID,
		AuthKey:                    authKey,
		SignKey:                    signKey,
		BlockCertificationRequest:  request,
		PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1},
	}
	return pr
}
