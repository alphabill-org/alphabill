package genesis

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
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
		wantErr    error
		wantErrStr string
	}{
		{
			name: "node identifier is empty",
			fields: fields{
				NodeIdentifier: "",
			},
			wantErr: ErrNodeIdentifierIsEmpty,
		},
		{
			name: "signing public key is missing",
			fields: fields{
				NodeIdentifier:   nodeIdentifier,
				SigningPublicKey: nil,
			},
			wantErr: ErrSigningPublicKeyIsInvalid,
		},
		{
			name: "signing public key is invalid",
			fields: fields{
				NodeIdentifier:   "1",
				SigningPublicKey: []byte{0, 0, 0, 0},
			},
			wantErrStr: "pubkey must be 33 bytes long",
		},
		{
			name: "encryption public key is missing",
			fields: fields{
				NodeIdentifier:      nodeIdentifier,
				SigningPublicKey:    pubKey,
				EncryptionPublicKey: nil,
			},
			wantErr: ErrEncryptionPublicKeyIsInvalid,
		},
		{
			name: "encryption public key is invalid",
			fields: fields{
				NodeIdentifier:      "1",
				SigningPublicKey:    pubKey,
				EncryptionPublicKey: []byte{0, 0, 0, 0},
			},
			wantErrStr: "pubkey must be 33 bytes long",
		},
		{
			name: "invalid p1 request",
			fields: fields{
				NodeIdentifier:                   nodeIdentifier,
				SigningPublicKey:                 pubKey,
				EncryptionPublicKey:              pubKey,
				BlockCertificationRequestRequest: nil,
			},
			wantErr: certification.ErrBlockCertificationRequestIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionNode{
				NodeIdentifier:            tt.fields.NodeIdentifier,
				SigningPublicKey:          tt.fields.SigningPublicKey,
				EncryptionPublicKey:       tt.fields.EncryptionPublicKey,
				BlockCertificationRequest: tt.fields.BlockCertificationRequestRequest,
			}
			err := x.IsValid()
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.ErrorContains(t, err, tt.wantErrStr)
			}
		})
	}
}

func TestPartitionNodeIsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	req := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   nodeIdentifier,
		RootRoundNumber:  1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: make([]byte, 32),
		},
	}
	require.NoError(t, req.Sign(signer))
	pn := &PartitionNode{
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
