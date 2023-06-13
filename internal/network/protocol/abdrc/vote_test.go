package abdrc

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/testutils"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

func TestVoteMsg_AddSignature(t *testing.T) {
	type fields struct {
		VoteInfo         *abtypes.RoundInfo
		LedgerCommitInfo *types.UnicitySeal
		HighQc           *abtypes.QuorumCert
		Author           string
	}
	type args struct {
		signer crypto.Signer
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	voteInfo := testutils.NewDummyRootRoundInfo(10)
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
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
				HighQc:           &abtypes.QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "",
		},
		{
			name: "Vote info hash is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: nil, Hash: nil},
				HighQc:           &abtypes.QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "invalid round info hash",
		},
		{
			name: "Signer is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: nil, Hash: nil},
				HighQc:           &abtypes.QuorumCert{},
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
		VoteInfo         *abtypes.RoundInfo
		LedgerCommitInfo *types.UnicitySeal
		HighQc           *abtypes.QuorumCert
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
	commitQcInfo := testutils.NewDummyRootRoundInfo(votedRound - 2)
	commitInfo := testutils.NewDummyCommitInfo(gocrypto.SHA256, commitQcInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	commitQc := &abtypes.QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	voteMsgInfo := testutils.NewDummyRootRoundInfo(votedRound)
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
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: nil, Hash: nil},
				HighQc:           &abtypes.QuorumCert{},
				Author:           "test",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, vote info is nil",
		},
		{
			name: "Vote info invalid",
			fields: fields{
				VoteInfo:         &abtypes.RoundInfo{RoundNumber: 8, ParentRoundNumber: 9},
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: nil, Hash: nil},
				HighQc:           &abtypes.QuorumCert{},
				Author:           "test",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, root round info round number is not valid",
		},
		{
			name: "Vote info hash error",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 1, 3}, Hash: nil},
				HighQc:           &abtypes.QuorumCert{},
				Author:           "test",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "vote info hash does not match hash in commit info",
		},
		{
			name: "Vote info no author",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: testutils.NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighQc:           &abtypes.QuorumCert{},
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
				LedgerCommitInfo: testutils.NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
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
				LedgerCommitInfo: testutils.NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighQc:           commitQc,
				Author:           "1",
				Signature:        nil,
			},
			args:       args{quorum: quorum, rootTrust: rootTrust},
			wantErrStr: "message signature verification failed",
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
	commitQcInfo := testutils.NewDummyRootRoundInfo(votedRound - 2)
	commitInfo := testutils.NewDummyCommitInfo(gocrypto.SHA256, commitQcInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	highQc := &abtypes.QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	voteMsgInfo := testutils.NewDummyRootRoundInfo(votedRound)
	voteMsg := &VoteMsg{
		VoteInfo: voteMsgInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			RootInternalInfo: voteMsgInfo.Hash(gocrypto.SHA256),
		},
		HighQc: highQc,
		Author: "1",
	}
	require.NoError(t, voteMsg.Sign(s1))
	require.NoError(t, voteMsg.Verify(quorum, rootTrust))
}
