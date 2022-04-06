package genesis

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
)

func TestPartitionNode_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, _ := verifier.MarshalPublicKey()
	type fields struct {
		NodeIdentifier string
		PublicKey      []byte
		P1Request      *p1.P1Request
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name: "node identifier is empty",
			fields: fields{
				NodeIdentifier: "",
				PublicKey:      pubKey,
				P1Request:      &p1.P1Request{},
			},
			wantErr: ErrNodeIdentifierIsEmpty,
		},
		{
			name: "node public key is nil",
			fields: fields{
				NodeIdentifier: "1",
				PublicKey:      nil,
				P1Request:      &p1.P1Request{},
			},
			wantErr: ErrPublicKeyIsInvalid,
		},
		{
			name: "node public key is invalid",
			fields: fields{
				NodeIdentifier: "1",
				PublicKey:      []byte{0, 0},
				P1Request:      &p1.P1Request{},
			},
			wantErr: errors.ErrInvalidArgument,
		},
		{
			name: "node P1 request is invalid",
			fields: fields{
				NodeIdentifier: "1",
				PublicKey:      pubKey,
				P1Request:      &p1.P1Request{},
			},
			wantErr: p1.ErrInvalidSystemIdentifierLength,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionNode{
				NodeIdentifier: tt.fields.NodeIdentifier,
				PublicKey:      tt.fields.PublicKey,
				P1Request:      tt.fields.P1Request,
			}
			err := x.IsValid()
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
