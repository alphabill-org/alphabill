package genesis

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

var systemDescription = &types.PartitionDescriptionRecord{Version: 1,
	NetworkIdentifier: 5,
	SystemIdentifier:  1,
	TypeIdLen:         8,
	UnitIdLen:         256,
	T2Timeout:         time.Second,
}

func TestPartitionRecord_IsValid(t *testing.T) {
	signingKey, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, encryptionPubKey := testsig.CreateSignerAndVerifier(t)
	type fields struct {
		SystemDescriptionRecord *types.PartitionDescriptionRecord
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
			wantErrStr: types.ErrSystemDescriptionIsNil.Error(),
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
				SystemDescriptionRecord: &types.PartitionDescriptionRecord{Version: 1,
					NetworkIdentifier: 5,
					SystemIdentifier:  2,
					TypeIdLen:         8,
					UnitIdLen:         256,
					T2Timeout:         time.Second,
				},
				Validators: []*PartitionNode{createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey)},
			},
			wantErrStr: "invalid system id: expected 00000002, got 00000001",
		},
		{
			name: "validators not unique",
			fields: fields{
				SystemDescriptionRecord: systemDescription,
				Validators: []*PartitionNode{
					createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey),
					createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey),
				},
			},
			wantErrStr: "validator list error, duplicated node id: 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionRecord{
				PartitionDescription: tt.fields.SystemDescriptionRecord,
				Validators:           tt.fields.Validators,
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
		PartitionDescription: systemDescription,
		Validators: []*PartitionNode{
			createPartitionNode(t, nodeIdentifier, signer, encryptionPubKey),
		},
	}
	require.NotNil(t, pr.GetPartitionNode(nodeIdentifier))
	require.Nil(t, pr.GetPartitionNode("2"))
}
