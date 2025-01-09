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
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	gpr := &GenesisPartitionRecord{
		Version:    1,
		Validators: []*PartitionNode{
			createPartitionNode(t, "1", signer),
		},
		PartitionDescription: &types.PartitionDescriptionRecord{
			Version:     1,
			NetworkID:   5,
			PartitionID: 2,
			TypeIDLen:   8,
			UnitIDLen:   256,
			T2Timeout:   time.Second,
		},
	}
	require.ErrorContains(t, gpr.IsValid(), "partition 00000002 node 1 has blockCertificationRequest for wrong partition 00000001")
}

func TestGenesisPartitionRecord_Verify(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)

	signingKey1, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signingKey2, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	type fields struct {
		Nodes                []*PartitionNode
		Certificate          *types.UnicityCertificate
		PartitionDescription *types.PartitionDescriptionRecord
	}
	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name:       "nodes missing",
			fields:     fields{},
			wantErrStr: ErrNodesAreMissing.Error(),
		},
		{
			name: "partition description record is nil",
			fields: fields{
				Nodes:                []*PartitionNode{nil},
				PartitionDescription: nil,
			},
			wantErrStr: types.ErrSystemDescriptionIsNil.Error(),
		},
		{
			name: "contains nodes with same node identifier",
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1),
					createPartitionNode(t, "1", signingKey2),
				},
				PartitionDescription: createPartitionDescriptionRecord(),
			},
			wantErrStr: "invalid partition nodes: duplicate node: 1",
		},
		{
			name: "contains nodes with same signing public key",
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1),
					createPartitionNode(t, "2", signingKey1),
				},
				PartitionDescription: createPartitionDescriptionRecord(),
			},
			wantErrStr: "invalid partition nodes: duplicate node signing key",
		},
		{
			name: "certificate is nil",
			fields: fields{
				Nodes: []*PartitionNode{
					createPartitionNode(t, "1", signingKey1),
				},
				PartitionDescription: createPartitionDescriptionRecord(),
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
			err = x.Verify(trustBase, crypto.SHA256)
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
	require.ErrorIs(t, pr.Verify(testtb.NewTrustBase(t, verifier), crypto.SHA256), ErrGenesisPartitionRecordIsNil)
}

func createPartitionDescriptionRecord() *types.PartitionDescriptionRecord{
	return &types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: 1,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   1 * time.Second,
	}
}

func createPartitionNode(t *testing.T, nodeID string, signer abcrypto.Signer) *PartitionNode {
	t.Helper()
	signVerifier, err := signer.Verifier()
	require.NoError(t, err)
	sigKey, err := signVerifier.MarshalPublicKey()
	require.NoError(t, err)

	request := &certification.BlockCertificationRequest{
		PartitionID: 1,
		NodeID:      nodeID,
		InputRecord: &types.InputRecord{
			Version:      1,
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    nil,
			SummaryValue: make([]byte, 32),
			RoundNumber:  1,
			Timestamp:    types.NewTimestamp(),
		},
	}
	require.NoError(t, request.Sign(signer))
	pr := &PartitionNode{
		Version:                    1,
		NodeID:                     nodeID,
		SigKey:                     sigKey,
		BlockCertificationRequest:  request,
		PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1},
	}
	return pr
}
