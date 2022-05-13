package genesis

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
	"github.com/stretchr/testify/require"
)

const nodeIdentifier = "1"

func TestPartitionNode_IsValid_InvalidInputs(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
		NodeIdentifier      string
		SigningPublicKey    []byte
		EncryptionPublicKey []byte
		P1Request           *p1.P1Request
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
				NodeIdentifier:      nodeIdentifier,
				SigningPublicKey:    pubKey,
				EncryptionPublicKey: pubKey,
				P1Request:           nil,
			},
			wantErr: p1.ErrP1RequestIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionNode{
				NodeIdentifier:      tt.fields.NodeIdentifier,
				SigningPublicKey:    tt.fields.SigningPublicKey,
				EncryptionPublicKey: tt.fields.EncryptionPublicKey,
				P1Request:           tt.fields.P1Request,
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
	p1 := &p1.P1Request{
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
	require.NoError(t, p1.Sign(signer))
	pn := &PartitionNode{
		NodeIdentifier:      nodeIdentifier,
		SigningPublicKey:    pubKey,
		EncryptionPublicKey: pubKey,
		P1Request:           p1,
	}
	require.NoError(t, pn.IsValid())
}

func TestPartitionNode_IsValid_PartitionNodeIsNil(t *testing.T) {
	var pn *PartitionNode
	require.ErrorIs(t, ErrPartitionNodeIsNil, pn.IsValid())
}
