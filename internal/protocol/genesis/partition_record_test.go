package genesis

import (
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"github.com/stretchr/testify/require"
)

var systemDescription = &SystemDescriptionRecord{
	SystemIdentifier: []byte{0, 0, 0, 0},
	T2Timeout:        10,
}

func TestPartitionRecord_IsValid(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	type fields struct {
		SystemDescriptionRecord *SystemDescriptionRecord
		Validators              []*PartitionNode
	}

	tests := []struct {
		name       string
		fields     fields
		wantErr    error
		wantErrStr string
	}{
		{
			name:    "system description record is nil",
			fields:  fields{},
			wantErr: ErrSystemDescriptionIsNil,
		},
		{
			name: "validators are missing",
			fields: fields{
				SystemDescriptionRecord: systemDescription,
			},
			wantErr: ErrValidatorsMissing,
		},
		{
			name: "validator is nil",
			fields: fields{
				SystemDescriptionRecord: systemDescription,
				Validators:              []*PartitionNode{nil},
			},
			wantErr: ErrPartitionNodeIsNil,
		},
		{
			name: "invalid validator system identifier",
			fields: fields{
				SystemDescriptionRecord: &SystemDescriptionRecord{
					SystemIdentifier: []byte{0, 0, 0, 1},
					T2Timeout:        10,
				},
				Validators: []*PartitionNode{createPartitionNode(t, nodeIdentifier, signer)},
			},
			wantErrStr: "invalid system id: expected 00000001, got 00000000",
		},
		{
			name: "validators not unique",
			fields: fields{
				SystemDescriptionRecord: systemDescription,
				Validators: []*PartitionNode{
					createPartitionNode(t, nodeIdentifier, signer),
					createPartitionNode(t, nodeIdentifier, signer),
				},
			},
			wantErrStr: "duplicated node id: 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionRecord{
				SystemDescriptionRecord: tt.fields.SystemDescriptionRecord,
				Validators:              tt.fields.Validators,
			}
			err := x.IsValid()
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.wantErrStr))
			}
		})
	}
}

func TestPartitionRecord_IsValid_Nil(t *testing.T) {
	var pr *PartitionRecord
	require.ErrorIs(t, ErrPartitionRecordIsNil, pr.IsValid())
}

func TestPartitionRecord_GetPartitionNode(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	pr := &PartitionRecord{
		SystemDescriptionRecord: systemDescription,
		Validators: []*PartitionNode{
			createPartitionNode(t, nodeIdentifier, signer),
		},
	}
	require.NotNil(t, pr.GetPartitionNode(nodeIdentifier))
	require.Nil(t, pr.GetPartitionNode("2"))
}
