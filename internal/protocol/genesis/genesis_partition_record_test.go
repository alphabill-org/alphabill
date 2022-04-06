package genesis

import (
	gocrypto "crypto"
	"testing"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

func TestGenesisPartitionRecord_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
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
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "nodes are missing",
			fields: fields{
				Nodes:                   nil,
				Certificate:             &certificates.UnicityCertificate{},
				SystemDescriptionRecord: &SystemDescriptionRecord{},
			},
			args: args{
				verifier:      verifier,
				hashAlgorithm: gocrypto.SHA256,
			},
			wantErr: ErrNodesAreMissing,
		},
		{
			name: "system description record is nil",
			fields: fields{
				Nodes:                   []*PartitionNode{nil},
				Certificate:             &certificates.UnicityCertificate{},
				SystemDescriptionRecord: nil,
			},
			args: args{
				verifier:      verifier,
				hashAlgorithm: gocrypto.SHA256,
			},
			wantErr: ErrSystemDescriptionIsNil,
		},
		{
			name: "invalid node",
			fields: fields{
				Nodes:                   []*PartitionNode{nil},
				Certificate:             &certificates.UnicityCertificate{},
				SystemDescriptionRecord: &SystemDescriptionRecord{SystemIdentifier: []byte{0, 0, 0, 0}, T2Timeout: 2},
			},
			args: args{
				verifier:      verifier,
				hashAlgorithm: gocrypto.SHA256,
			},
			wantErr: ErrPartitionNodeIsNil,
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
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
