package atomic_broadcast

import (
	"crypto"
	"crypto/sha256"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/stretchr/testify/require"
)

func TestBlockData_Hash(t *testing.T) {
	block := &BlockData{
		Author:    "test",
		Round:     2,
		Epoch:     0,
		Timestamp: 0x0102030405060708,
		Payload:   &Payload{},
		Qc: &QuorumCert{
			VoteInfo: &VoteInfo{
				BlockId:       []byte{0, 1, 1},
				RootRound:     2,
				Epoch:         0,
				Timestamp:     0x0010670314583523,
				ParentBlockId: []byte{0, 1},
				ParentRound:   1,
				ExecStateId:   []byte{0, 1, 3}},
			LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 1, 2}, CommitStateId: []byte{1, 2, 3}},
			Signatures:       map[string][]byte{"1": {1, 2, 3}, "2": {1, 2, 4}, "3": {1, 2, 5}},
		},
	}
	serializedBlock := []byte{
		't', 'e', 's', 't',
		0, 0, 0, 0, 0, 0, 0, 2,
		0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		// Empty payload
		1, 2, 3, // "1" QC signature (in alphabetical order)
		1, 2, 4, // "2" QC signature
		1, 2, 5, // "3" QC signature
	}
	expected := sha256.Sum256(serializedBlock)
	hash, err := block.Hash(crypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, expected[:], hash)
	// Block id is hash and not included in hashing
	block.Id = hash
	hash2, err := block.Hash(crypto.SHA256)
	require.Equal(t, hash, hash2)
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
		quorum    uint32
		rootTrust map[string]abcrypto.Verifier
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
			args:       args{quorum: 1, rootTrust: nil},
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
			args:       args{quorum: 1, rootTrust: nil},
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
			args:       args{quorum: 1, rootTrust: nil},
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
			args:       args{quorum: 1, rootTrust: nil},
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
			err := x.Verify(tt.args.quorum, tt.args.rootTrust)
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
	tests := []struct {
		name       string
		fields     fields
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
			err := x.IsValid()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
