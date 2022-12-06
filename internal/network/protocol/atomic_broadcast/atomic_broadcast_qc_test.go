package atomic_broadcast

import (
	gocrypto "crypto"
	"crypto/sha256"
	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
	"hash"
	"testing"
)

var dummyVoteInfo = &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: 10, Epoch: 0,
	Timestamp: 9, ParentBlockId: []byte{0, 1}, ParentRound: 9, ExecStateId: []byte{0, 1, 3}}

func NewDummyVoteInfo(round, parentRound uint64) *VoteInfo {
	return &VoteInfo{BlockId: []byte{0, 1, 1}, RootRound: round, Epoch: 0,
		Timestamp: 1670314583523, ParentBlockId: []byte{0, 1}, ParentRound: parentRound, ExecStateId: []byte{0, 1, 3}}
}

func NewDummyCommitInfo(algo gocrypto.Hash, voteInfo *VoteInfo) *LedgerCommitInfo {
	hash := voteInfo.Hash(algo)
	return &LedgerCommitInfo{VoteInfoHash: hash, CommitStateId: nil}
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
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, dummyVoteInfo),
				Signatures:       nil,
			},
			args: args{hasher: sha256.New()},
		},
		{
			name: "Hash QC - with signatures",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, dummyVoteInfo),
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
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, dummyVoteInfo),
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
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, dummyVoteInfo),
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
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, dummyVoteInfo),
				Signatures:       nil,
			},
			wantErrStr: ErrQcIsMissingSignatures.Error(),
		},
		// Is valid, but would not verify
		{
			name: "QC valid - would not pass verify",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, dummyVoteInfo),
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
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	commitInfo := NewDummyCommitInfo(gocrypto.SHA256, dummyVoteInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)

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
				LedgerCommitInfo: commitInfo,
				Signatures:       nil,
			},
			wantErrStr: "qc is missing signatures",
		},
		{
			name: "QC signatures not valid",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": {0, 1, 2}, "2": {0, 1, 2}, "3": {0, 1, 2}},
			},
			args:       args{quorum: 2, rootTrust: rootTrust},
			wantErrStr: "quorum certificate not valid: signature length is",
		},
		{
			name: "QC no quorum",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": {0, 1, 2}, "2": {0, 1, 2}},
			},
			args:       args{quorum: 3, rootTrust: rootTrust},
			wantErrStr: "less than quorum",
		},
		{
			name: "QC invalid signature means no qc is not valid",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": {0, 1, 2}},
			},
			args:       args{quorum: 2, rootTrust: rootTrust},
			wantErrStr: "quorum certificate not valid: signature length is",
		},
		{
			name: "QC is valid",
			fields: fields{
				VoteInfo:         dummyVoteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
			},
			args: args{quorum: 2, rootTrust: rootTrust},
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
