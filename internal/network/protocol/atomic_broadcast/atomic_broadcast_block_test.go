package atomic_broadcast

import (
	"crypto"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlockData_Hash(t *testing.T) {
	type fields struct {
		Id        []byte
		Author    string
		Round     uint64
		Epoch     uint64
		Timestamp uint64
		Payload   *Payload
		Qc        *QuorumCert
	}
	type args struct {
		algo crypto.Hash
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Hash all fields",
			fields: fields{
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 1, ExecStateId: []byte{0, 1, 3}},
					LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 2, 1}},
				},
			},
			args: args{
				algo: crypto.SHA256,
			},
			want:    []byte{141, 97, 153, 122, 191, 167, 167, 46, 135, 120, 31, 49, 190, 238, 47, 255, 222, 92, 173, 176, 73, 179, 253, 165, 179, 1, 127, 44, 33, 184, 171, 39},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &BlockData{
				Id:        tt.fields.Id,
				Author:    tt.fields.Author,
				Round:     tt.fields.Round,
				Epoch:     tt.fields.Epoch,
				Timestamp: tt.fields.Timestamp,
				Payload:   tt.fields.Payload,
				Qc:        tt.fields.Qc,
			}
			got, err := x.Hash(tt.args.algo)
			if (err != nil) != tt.wantErr {
				t.Errorf("Hash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Hash() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlockData_IsValid(t *testing.T) {
	type fields struct {
		Id        []byte
		Author    string
		Round     uint64
		Epoch     uint64
		Timestamp uint64
		Payload   *Payload
		Qc        *QuorumCert
	}
	type args struct {
		p PartitionVerifier
		v AtomicVerifier
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr error
	}{
		{
			name: "Invalid block id",
			fields: fields{
				Id:        nil,
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 1, ExecStateId: []byte{0, 1, 3}},
					LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 2, 1}},
				},
			},
			args:       args{p: nil, v: nil},
			wantErrStr: ErrInvalidBlockId,
		},
		{
			name: "Invalid round number",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     0,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 1, ExecStateId: []byte{0, 1, 3}},
					LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 2, 1}},
				},
			},
			args:       args{p: nil, v: nil},
			wantErrStr: ErrInvalidRound,
		},
		{
			name: "Invalid payload",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   nil, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 1, ExecStateId: []byte{0, 1, 3}},
					LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 2, 1}},
				},
			},
			args:       args{p: nil, v: nil},
			wantErrStr: ErrMissingPayload,
		},
		{
			name: "Invalid QC",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc:        nil,
			},
			args:       args{p: nil, v: nil},
			wantErrStr: ErrMissingQuorumCertificate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &BlockData{
				Id:        tt.fields.Id,
				Author:    tt.fields.Author,
				Round:     tt.fields.Round,
				Epoch:     tt.fields.Epoch,
				Timestamp: tt.fields.Timestamp,
				Payload:   tt.fields.Payload,
				Qc:        tt.fields.Qc,
			}
			err := x.IsValid(tt.args.p, tt.args.v)
			require.ErrorIs(t, err, tt.wantErrStr)
		})
	}
}

func TestPayload_IsEmpty(t *testing.T) {
	type fields struct {
		Requests []*IRChangeReqMsg
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name:   "empty (nil)",
			fields: fields{Requests: nil},
			want:   true,
		},
		{
			name:   "empty",
			fields: fields{Requests: []*IRChangeReqMsg{}},
			want:   true,
		},
		{
			name: "not empty",
			fields: fields{Requests: []*IRChangeReqMsg{
				{SystemIdentifier: []byte{0, 0, 0, 1}, CertReason: IRChangeReqMsg_T2_TIMEOUT},
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &Payload{
				Requests: tt.fields.Requests,
			}
			if got := x.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPayload_IsValid(t *testing.T) {
	type fields struct {
		Requests []*IRChangeReqMsg
	}
	type args struct {
		partitions PartitionVerifier
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name:       "empty (nil)",
			fields:     fields{Requests: nil},
			wantErrStr: "",
		},
		{
			name:       "empty",
			fields:     fields{Requests: []*IRChangeReqMsg{}},
			wantErrStr: "",
		},
		{
			name: "valid timeout",
			fields: fields{Requests: []*IRChangeReqMsg{
				{SystemIdentifier: []byte{0, 0, 0, 1}, CertReason: IRChangeReqMsg_T2_TIMEOUT},
			}},
			wantErrStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &Payload{
				Requests: tt.fields.Requests,
			}
			err := x.IsValid(tt.args.partitions)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
