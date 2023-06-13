package abdrc

import (
	"crypto"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

func TestIRChangeReqMsg_AddToHasher(t *testing.T) {
	msg := &IRChangeReqMsg{
		IRChangeReq: &abtypes.IRChangeReq{
			SystemIdentifier: []byte{1, 2, 3, 4},
			CertReason:       abtypes.T2Timeout,
			Requests:         []*certification.BlockCertificationRequest{},
		},
	}
	hasher := crypto.SHA256.New()
	msg.AddToHasher(hasher)
	require.NotNil(t, hasher.Sum(nil))
}

func TestIRChangeReqMsg_GetSystemID(t *testing.T) {
	type fields struct {
		IRChangeReq *abtypes.IRChangeReq
	}
	tests := []struct {
		name   string
		fields fields
		want   types.SystemID
	}{
		{
			name: "ok",
			fields: fields{
				IRChangeReq: &abtypes.IRChangeReq{
					SystemIdentifier: []byte{1, 2, 3, 4},
					CertReason:       abtypes.T2Timeout,
					Requests:         []*certification.BlockCertificationRequest{},
				},
			},
			want: []byte{1, 2, 3, 4},
		},
		{
			name: "IR Change request is nil",
			fields: fields{
				IRChangeReq: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				IRChangeReq: tt.fields.IRChangeReq,
			}
			if got := x.GetSystemID(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetSystemID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIRChangeReqMsg_IsValid(t *testing.T) {
	type fields struct {
		IRChangeReq *abtypes.IRChangeReq
	}
	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name: "ok",
			fields: fields{
				IRChangeReq: &abtypes.IRChangeReq{
					SystemIdentifier: []byte{1, 2, 3, 4},
					CertReason:       abtypes.T2Timeout,
					Requests:         []*certification.BlockCertificationRequest{},
				},
			},
		},
		{
			name: "err - missing payload",
			fields: fields{
				IRChangeReq: nil,
			},
			wantErrStr: "ir change request is missing payload",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				IRChangeReq: tt.fields.IRChangeReq,
			}
			err := x.IsValid()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIRChangeReqMsg_Verify(t *testing.T) {
	type fields struct {
		IRChangeReq *abtypes.IRChangeReq
	}
	type args struct {
		tb         partitions.PartitionTrustBase
		luc        *types.UnicityCertificate
		round      uint64
		t2InRounds uint64
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		want       *types.InputRecord
		wantErrStr string
	}{
		{
			name: "err - tb is nil",
			fields: fields{
				IRChangeReq: &abtypes.IRChangeReq{
					SystemIdentifier: []byte{1, 2, 3, 4},
					CertReason:       abtypes.T2Timeout,
					Requests:         []*certification.BlockCertificationRequest{},
				},
			},
			args:       args{tb: nil, luc: nil, round: 1, t2InRounds: 1},
			wantErrStr: "partition info is nil",
		},
		{
			name: "verify error",
			fields: fields{
				IRChangeReq: nil,
			},
			args:       args{tb: nil, luc: nil, round: 1, t2InRounds: 1},
			wantErrStr: "ir change request is missing payload",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				IRChangeReq: tt.fields.IRChangeReq,
			}
			_, err := x.Verify(tt.args.tb, tt.args.luc, tt.args.round, tt.args.t2InRounds)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
