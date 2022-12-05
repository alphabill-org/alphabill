package atomic_broadcast

import (
	"crypto/sha256"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/stretchr/testify/require"
	"hash"
	"testing"
)

var dummyVoteInfo = &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 2, Epoch: 0,
	Timestamp: 9, ParentBlockId: []byte{0, 1}, ParentRound: 1, ExecStateId: []byte{0, 1, 3}}

func NewDummyCommitInfo(hasher hash.Hash, voteInfo *VoteInfo) *LedgerCommitInfo {
	voteInfo.AddToHasher(hasher)
	return &LedgerCommitInfo{VoteInfoHash: hasher.Sum(nil), CommitStateId: nil}
}

func TestNewQuorumCertificate(t *testing.T) {
	voteInfo := &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 1, ExecStateId: []byte{0, 1, 3}}
	commitInfo := &LedgerCommitInfo{VoteInfoHash: []byte{0, 1, 2}}
	qc := NewQuorumCertificate(voteInfo, commitInfo, nil)
	require.NotNil(t, qc)
}

func TestQuorumCert_AddToHasher(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		Signatures       map[string][]byte
	}
	type args struct {
		hasher hash.Hash
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "Hash QC - no signatures",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(sha256.New(), dummyVoteInfo),
				Signatures:       nil,
			},
			args: args{hasher: sha256.New()},
		},
		{
			name: "Hash QC - with signatures",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(sha256.New(), dummyVoteInfo),
				Signatures: map[string][]byte{
					"test1": {0, 1, 2},
					"test2": {1, 2, 3},
				},
			},
			args: args{hasher: sha256.New()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &QuorumCert{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				Signatures:       tt.fields.Signatures,
			}
			x.AddToHasher(tt.args.hasher)
		})
	}
}

func TestQuorumCert_IsValid(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		Signatures       map[string][]byte
	}
	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name: "QC not  valid - no vote info",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: NewDummyCommitInfo(sha256.New(), dummyVoteInfo),
				Signatures:       nil,
			},
			wantErrStr: ErrVoteInfoIsNil.Error(),
		},
		// Vote info valid unit tests are covered in VoteInfo tests
		{
			name: "QC not  valid - vote info no valid",
			fields: fields{
				VoteInfo: &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 2, Epoch: 0,
					Timestamp: 9, ParentBlockId: []byte{0, 1}, ParentRound: 2, ExecStateId: []byte{0, 1, 3}},
				LedgerCommitInfo: NewDummyCommitInfo(sha256.New(), dummyVoteInfo),
				Signatures:       nil,
			},
			wantErrStr: "invalid round number",
		},
		{
			name: "QC not  valid - ledger info is nil",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: nil,
				Signatures:       nil,
			},
			wantErrStr: ErrLedgerCommitInfoIsNil.Error(),
		},
		{
			name: "QC not  valid - ledger info is nil",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				Signatures:       nil,
			},
			wantErrStr: ErrInvalidVoteInfoHash.Error(),
		},
		{
			name: "QC not  valid - missing signatures",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(sha256.New(), dummyVoteInfo),
				Signatures:       nil,
			},
			wantErrStr: ErrMissingSignatures.Error(),
		},
		// Is valid, but would not verify
		{
			name: "QC valid - would not pass verify",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(sha256.New(), dummyVoteInfo),
				Signatures: map[string][]byte{
					"test1": {0, 1, 2},
					"test2": {1, 2, 3},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &QuorumCert{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				Signatures:       tt.fields.Signatures,
			}
			err := x.IsValid()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestQuorumCert_Verify(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		Signatures       map[string][]byte
	}
	type args struct {
		quorum    uint32
		rootTrust map[string]crypto.Verifier
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "QC not  valid - missing signatures",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(sha256.New(), dummyVoteInfo),
				Signatures:       nil,
			},
			wantErrStr: "qc is missing signatures",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &QuorumCert{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				Signatures:       tt.fields.Signatures,
			}
			err := x.Verify(tt.args.quorum, tt.args.rootTrust)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
		})
	}
}
