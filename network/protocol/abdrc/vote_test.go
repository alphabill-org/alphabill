package abdrc

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/testutils"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestVoteMsg_AddSignature(t *testing.T) {
	type fields struct {
		VoteInfo         *drctypes.RoundInfo
		LedgerCommitInfo *types.UnicitySeal
		HighQc           *drctypes.QuorumCert
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
				LedgerCommitInfo: &types.UnicitySeal{PreviousHash: voteInfo.Hash(gocrypto.SHA256)},
				HighQc:           &drctypes.QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "",
		},
		{
			name: "Vote info hash is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &types.UnicitySeal{PreviousHash: nil, Hash: nil},
				HighQc:           &drctypes.QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "invalid round info hash",
		},
		{
			name: "Signer is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &types.UnicitySeal{PreviousHash: nil, Hash: nil},
				HighQc:           &drctypes.QuorumCert{},
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

func Test_VoteMsg_Verify(t *testing.T) {
	const votedRound = 10
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

	// valid vote obj - each test creates copy of it to make single field invalid
	validVoteMsg := func(t *testing.T) *VoteMsg {
		t.Helper()
		voteMsgInfo := testutils.NewDummyRootRoundInfo(votedRound)
		vote := &VoteMsg{
			VoteInfo: voteMsgInfo,
			LedgerCommitInfo: &types.UnicitySeal{
				PreviousHash: voteMsgInfo.Hash(gocrypto.SHA256),
			},
			HighQc: &drctypes.QuorumCert{
				VoteInfo:         commitQcInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
			},
			Author: "1",
		}
		require.NoError(t, vote.Sign(s1))
		return vote
	}
	require.NoError(t, validVoteMsg(t).Verify(uint32(len(rootTrust)), rootTrust), "expected validVoteMsg to return valid vote struct")

	t.Run("VoteInfo is missing", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.VoteInfo = nil
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' is missing vote info`)
	})

	t.Run("invalid vote info", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.VoteInfo.RoundNumber = 0
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' vote info error: round number is not assigned`)
	})

	t.Run("missing US", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.LedgerCommitInfo = nil
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' ledger commit info (unicity seal) is missing`)
	})

	t.Run("invalid vote info hash", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.VoteInfo.Epoch += 1
		require.ErrorContains(t, vi.Verify(3, rootTrust), `vote from '1' vote info hash does not match hash in commit info`)

		vi.VoteInfo.Epoch -= 1
		vi.LedgerCommitInfo.PreviousHash = nil
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' vote info hash does not match hash in commit info`)
	})

	t.Run("high QC is missing", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.HighQc = nil
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' high QC is nil`)
	})

	t.Run("invalid high QC", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.HighQc.VoteInfo = nil
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' high QC error: invalid quorum certificate: vote info is nil`)
	})

	t.Run("unassigned author", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.Author = ""
		require.EqualError(t, vi.Verify(3, rootTrust), `author is missing`)
	})

	t.Run("missing author", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.Author = "unknown"
		require.EqualError(t, vi.Verify(3, rootTrust), `author 'unknown' is not in the trustbase`)
	})

	t.Run("invalid signature", func(t *testing.T) {
		vi := validVoteMsg(t)
		vi.Signature[0] = vi.Signature[0] + 1
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' signature verification error: verification failed`)

		vi.Signature = []byte{0, 1, 2, 3, 4}
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' signature verification error: signature length is 5 b (expected 64 b)`)

		vi.Signature = nil
		require.EqualError(t, vi.Verify(3, rootTrust), `vote from '1' signature verification error: invalid nil argument`)
	})
}
