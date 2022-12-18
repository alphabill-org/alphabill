package atomic_broadcast

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestVoteMsg_AddSignature(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		HighQc           *QuorumCert
		Author           string
	}
	type args struct {
		signer crypto.Signer
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	voteInfo := NewDummyVoteInfo(10)
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Sign ok",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteInfo),
				HighQc:           &QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "",
		},
		{
			name: "Vote info hash is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighQc:           &QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "invalid vote info hash",
		},
		{
			name: "Signer is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighQc:           &QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: nil},
			wantErrStr: "signer is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &VoteMsg{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				HighQc:           tt.fields.HighQc,
				Author:           tt.fields.Author,
			}
			err := x.Sign(tt.args.signer)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVoteMsg_Verify(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		HighQc           *QuorumCert
		Author           string
		Signature        []byte
	}
	type args struct {
		quorum    uint32
		rootTrust map[string]crypto.Verifier
	}
	const votedRound = 10
	const quorum = 3
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	commitQcInfo := NewDummyVoteInfo(votedRound - 2)
	commitInfo := NewDummyCommitInfo(gocrypto.SHA256, commitQcInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	commitQc := &QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	voteMsgInfo := NewDummyVoteInfo(votedRound)
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Vote info error",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighQc:           &QuorumCert{},
				Author:           "test",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, vote info is nil",
		},
		{
			name: "Vote info invalid",
			fields: fields{
				VoteInfo:         &VoteInfo{RootRound: 8, ParentRound: 9},
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighQc:           &QuorumCert{},
				Author:           "test",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, invalid round number",
		},
		{
			name: "Vote info hash error",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 1, 3}, CommitStateId: nil},
				HighQc:           &QuorumCert{},
				Author:           "test",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, vote info hash verification failed",
		},
		{
			name: "Vote info no author",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighQc:           &QuorumCert{},
				Author:           "",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: rootTrust},
			wantErrStr: "invalid vote message, no author",
		},
		{
			name: "Vote info author public key not registered",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighQc:           commitQc,
				Author:           "test",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: rootTrust},
			wantErrStr: "failed to find public key for author test",
		},
		{
			name: "Vote info author public key not registered",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighQc:           commitQc,
				Author:           "1",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: rootTrust},
			wantErrStr: "signature verification failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &VoteMsg{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				HighQc:           tt.fields.HighQc,
				Author:           tt.fields.Author,
				Signature:        tt.fields.Signature,
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

func TestVoteMsg_VerifyOk(t *testing.T) {
	const votedRound = 10
	const quorum = 3
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	commitQcInfo := NewDummyVoteInfo(votedRound - 2)
	commitInfo := NewDummyCommitInfo(gocrypto.SHA256, commitQcInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	highQc := &QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	voteMsgInfo := NewDummyVoteInfo(votedRound)
	voteMsg := &VoteMsg{
		VoteInfo:         voteMsgInfo,
		LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
		HighQc:           highQc,
		Author:           "1",
	}
	require.NoError(t, voteMsg.Sign(s1))
	require.NoError(t, voteMsg.Verify(quorum, rootTrust))
}
