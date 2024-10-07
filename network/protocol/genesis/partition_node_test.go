package genesis

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/stretchr/testify/require"
)

const nodeIdentifier = "1"

func TestPartitionNode_IsValid_InvalidInputs(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
		NodeIdentifier                   string
		SigningPublicKey                 []byte
		EncryptionPublicKey              []byte
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
				NodeIdentifier: "",
			},
			wantErrStr: ErrNodeIdentifierIsEmpty.Error(),
		},
		{
			name: "signing public key is missing",
			fields: fields{
				NodeIdentifier:   nodeIdentifier,
				SigningPublicKey: nil,
			},
			wantErrStr: ErrSigningPublicKeyIsInvalid.Error(),
		},
		{
			name: "signing public key is invalid",
			fields: fields{
				NodeIdentifier:   "1",
				SigningPublicKey: []byte{0, 0, 0, 0},
			},
			wantErrStr: "invalid signing public key, pubkey must be 33 bytes long, but is 4",
		},
		{
			name: "encryption public key is missing",
			fields: fields{
				NodeIdentifier:      nodeIdentifier,
				SigningPublicKey:    pubKey,
				EncryptionPublicKey: nil,
			},
			wantErrStr: ErrEncryptionPublicKeyIsInvalid.Error(),
		},
		{
			name: "encryption public key is invalid",
			fields: fields{
				NodeIdentifier:      "1",
				SigningPublicKey:    pubKey,
				EncryptionPublicKey: []byte{0, 0, 0, 0},
			},
			wantErrStr: "invalid encryption public key, pubkey must be 33 bytes long, but is 4",
		},
		{
			name: "invalid p1 request",
			fields: fields{
				NodeIdentifier:                   nodeIdentifier,
				SigningPublicKey:                 pubKey,
				EncryptionPublicKey:              pubKey,
				BlockCertificationRequestRequest: nil,
			},
			wantErrStr: "block certification request validation failed, block certification request is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionNode{
				Version:                   1,
				NodeIdentifier:            tt.fields.NodeIdentifier,
				SigningPublicKey:          tt.fields.SigningPublicKey,
				EncryptionPublicKey:       tt.fields.EncryptionPublicKey,
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
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	req := &certification.BlockCertificationRequest{
		Partition:      1,
		NodeIdentifier: nodeIdentifier,
		InputRecord: &types.InputRecord{
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: make([]byte, 32),
			RoundNumber:  1,
		},
		RootRoundNumber: 1,
	}
	require.NoError(t, req.Sign(signer))
	pn := &PartitionNode{
		Version:                   1,
		NodeIdentifier:            nodeIdentifier,
		SigningPublicKey:          pubKey,
		EncryptionPublicKey:       pubKey,
		BlockCertificationRequest: req,
	}
	require.NoError(t, pn.IsValid())
}

func TestPartitionNode_IsValid_PartitionNodeIsNil(t *testing.T) {
	var pn *PartitionNode
	require.ErrorIs(t, ErrPartitionNodeIsNil, pn.IsValid())
}
