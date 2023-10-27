package genesis

import (
	"testing"

	"github.com/alphabill-org/alphabill/api/genesis"
	"github.com/alphabill-org/alphabill/common/crypto"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"

	"github.com/stretchr/testify/require"
)

var systemDescription = &genesis.SystemDescriptionRecord{
	SystemIdentifier: []byte{0, 0, 0, 0},
	T2Timeout:        10,
}

func TestPartitionRecord_IsValid(t *testing.T) {
	signingKey, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, encryptionPubKey := testsig.CreateSignerAndVerifier(t)
	type fields struct {
		SystemDescriptionRecord *genesis.SystemDescriptionRecord
		Validators              []*PartitionNode
	}

	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name:       "system description record is nil",
			fields:     fields{},
			wantErrStr: genesis.ErrSystemDescriptionIsNil.Error(),
		},
		{
			name: "validators are missing",
			fields: fields{
				SystemDescriptionRecord: systemDescription,
			},
			wantErrStr: errValidatorsMissing.Error(),
		},
		{
			name: "validator is nil",
			fields: fields{
				SystemDescriptionRecord: systemDescription,
				Validators:              []*PartitionNode{nil},
			},
			wantErrStr: "validators list error, partition node is nil",
		},
		{
			name: "invalid validator system identifier",
			fields: fields{
				SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 1},
					T2Timeout:        10,
				},
				Validators: []*PartitionNode{genesis.createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey)},
			},
			wantErrStr: "invalid system id: expected 00000001, got 00000000",
		},
		{
			name: "validators not unique",
			fields: fields{
				SystemDescriptionRecord: systemDescription,
				Validators: []*PartitionNode{
					genesis.createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey),
					genesis.createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey),
				},
			},
			wantErrStr: "validator list error, duplicated node id: 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionRecord{
				SystemDescriptionRecord: tt.fields.SystemDescriptionRecord,
				Validators:              tt.fields.Validators,
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

func TestPartitionRecord_IsValid_Nil(t *testing.T) {
	var pr *PartitionRecord
	require.ErrorIs(t, errPartitionRecordIsNil, pr.IsValid())
}

func TestPartitionRecord_GetPartitionNode(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, encryptionPubKey := testsig.CreateSignerAndVerifier(t)
	pr := &PartitionRecord{
		SystemDescriptionRecord: systemDescription,
		Validators: []*PartitionNode{
			genesis.createPartitionNode(t, nodeIdentifier, signer, encryptionPubKey),
		},
	}
	require.NotNil(t, pr.GetPartitionNode(nodeIdentifier))
	require.Nil(t, pr.GetPartitionNode("2"))
}
