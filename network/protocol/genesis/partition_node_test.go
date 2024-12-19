package genesis

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/stretchr/testify/require"
)

const nodeID = "1"

func TestPartitionNode_IsValid_InvalidInputs(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
		NodeID                           string
		SignKey                          []byte
		AuthKey                          []byte
		BlockCertificationRequestRequest *certification.BlockCertificationRequest
	}

	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name: "node identifier is empty",
			fields: fields{
				NodeID: "",
			},
			wantErrStr: ErrNodeIDIsEmpty.Error(),
		},
		{
			name: "signing public key is missing",
			fields: fields{
				NodeID:  nodeID,
				SignKey: nil,
			},
			wantErrStr: ErrSignKeyIsInvalid.Error(),
		},
		{
			name: "signing public key is invalid",
			fields: fields{
				NodeID:  "1",
				SignKey: []byte{0, 0, 0, 0},
			},
			wantErrStr: "invalid signing public key, pubkey must be 33 bytes long, but is 4",
		},
		{
			name: "authentication public key is missing",
			fields: fields{
				NodeID:  nodeID,
				SignKey: pubKey,
				AuthKey: nil,
			},
			wantErrStr: ErrAuthKeyIsInvalid.Error(),
		},
		{
			name: "authentication public key is invalid",
			fields: fields{
				NodeID:  "1",
				SignKey: pubKey,
				AuthKey: []byte{0, 0, 0, 0},
			},
			wantErrStr: "invalid authentication public key, pubkey must be 33 bytes long, but is 4",
		},
		{
			name: "invalid p1 request",
			fields: fields{
				NodeID:                           nodeID,
				SignKey:                          pubKey,
				AuthKey:                          pubKey,
				BlockCertificationRequestRequest: nil,
			},
			wantErrStr: "block certification request validation failed, block certification request is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionNode{
				Version:                   1,
				NodeID:                    tt.fields.NodeID,
				AuthKey:                   tt.fields.AuthKey,
				SignKey:                   tt.fields.SignKey,
				BlockCertificationRequest: tt.fields.BlockCertificationRequestRequest,
			}
			err = x.IsValid()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPartitionNodeIsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	signKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	req := &certification.BlockCertificationRequest{
		PartitionID: 1,
		NodeID:      nodeID,
		InputRecord: &types.InputRecord{
			Version:      1,
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: make([]byte, 32),
			Timestamp:    types.NewTimestamp(),
			RoundNumber:  1,
		},
	}
	require.NoError(t, req.Sign(signer))
	pn := &PartitionNode{
		Version:                   1,
		NodeID:                    nodeID,
		AuthKey:                   signKey,
		SignKey:                   signKey,
		BlockCertificationRequest: req,
	}
	require.NoError(t, pn.IsValid())
}

func TestPartitionNode_IsValid_PartitionNodeIsNil(t *testing.T) {
	var pn *PartitionNode
	require.ErrorIs(t, ErrPartitionNodeIsNil, pn.IsValid())
}
