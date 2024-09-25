package abdrc

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/testutils"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/stretchr/testify/require"
)

func addQCSignature(t *testing.T, qc *drctypes.QuorumCert, author string, signer crypto.Signer) {
	t.Helper()
	sig, err := signer.SignBytes(qc.LedgerCommitInfo.Bytes())
	require.NoError(t, err)
	qc.Signatures[author] = sig
}

func TestProposalMsg_IsValid(t *testing.T) {
	type fields struct {
		Block  *drctypes.BlockData
		LastTc *drctypes.TimeoutCert
	}
	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name: "Block is nil",
			fields: fields{
				Block: nil,
			},
			wantErrStr: "block is nil",
		},
		{
			name: "Block not valid",
			fields: fields{
				Block: &drctypes.BlockData{
					Author:    "",
					Round:     0,
					Epoch:     0,
					Timestamp: 0,
					Payload:   nil,
					Qc:        nil,
				},
			},
			wantErrStr: "invalid block: round number is not assigned",
		},
		{
			name: "Invalid block round, does not follow QC",
			fields: fields{
				Block: &drctypes.BlockData{
					Author:    "test",
					Round:     9,
					Epoch:     0,
					Timestamp: 1670314583525,
					Payload:   &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo:         testutils.NewDummyRootRoundInfo(7),
						LedgerCommitInfo: testutils.NewDummyCommitInfo(gocrypto.SHA256, testutils.NewDummyRootRoundInfo(7)),
						Signatures:       map[string][]byte{"test": {0, 1, 2, 3}},
					},
				},
			},
			wantErrStr: "proposed block round 9 does not follow attached quorum certificate round 7",
		},
		{
			name: "Invalid block round, does not follow QC nor TC",
			fields: fields{
				Block: &drctypes.BlockData{
					Author:    "test",
					Round:     9,
					Epoch:     0,
					Timestamp: 1670314583525,
					Payload:   &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo:         testutils.NewDummyRootRoundInfo(5),
						LedgerCommitInfo: testutils.NewDummyCommitInfo(gocrypto.SHA256, testutils.NewDummyRootRoundInfo(5)),
						Signatures:       map[string][]byte{"test": {0, 1, 2, 3}},
					},
				},
				LastTc: &drctypes.TimeoutCert{
					Timeout: &drctypes.Timeout{
						Epoch:  0,
						Round:  7,
						HighQc: nil,
					},
				},
			},
			wantErrStr: "proposed block round 9 does not follow attached quorum certificate round 7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &ProposalMsg{
				Block:       tt.fields.Block,
				LastRoundTc: tt.fields.LastTc, // timeout certificate is optional
				Signature:   nil,              // IsValid does not check signatures
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

func TestProposalMsg_Sign_SignerIsNil(t *testing.T) {
	prevRoundInfo := &drctypes.RoundInfo{RoundNumber: 8}
	proposeMsg := &ProposalMsg{
		Block: &drctypes.BlockData{
			Author:    "1",
			Round:     9,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &drctypes.Payload{},
			Qc: &drctypes.QuorumCert{
				VoteInfo: prevRoundInfo,
				LedgerCommitInfo: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
					seal.PreviousHash = prevRoundInfo.Hash(gocrypto.SHA256)
				}),
				Signatures: map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	require.ErrorIs(t, errSignerIsNil, proposeMsg.Sign(nil))
}

func TestProposalMsg_Sign_InvalidBlock(t *testing.T) {
	qcInfo := &drctypes.RoundInfo{RoundNumber: 8}

	proposeMsg := &ProposalMsg{
		Block: &drctypes.BlockData{
			Author:    "1",
			Round:     0,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &drctypes.Payload{},
			Qc: &drctypes.QuorumCert{
				VoteInfo: qcInfo,
				LedgerCommitInfo: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
					seal.PreviousHash = qcInfo.Hash(gocrypto.SHA256)
				}),
				Signatures: map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	require.ErrorContains(t, proposeMsg.Sign(s1), "round number is not assigned")
}

func TestProposalMsg_Sign_Ok(t *testing.T) {
	qcInfo := testutils.NewDummyRootRoundInfo(9)

	proposeMsg := &ProposalMsg{
		Block: &drctypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &drctypes.Payload{},
			Qc: &drctypes.QuorumCert{
				VoteInfo: qcInfo,
				LedgerCommitInfo: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
					seal.PreviousHash = qcInfo.Hash(gocrypto.SHA256)
				}),
				Signatures: map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	require.NoError(t, proposeMsg.Sign(s1))
}

func TestProposalMsg_Verify(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := testtb.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3})

	validProposal := func(t *testing.T) *ProposalMsg {
		t.Helper()
		voteInfo := testutils.NewDummyRootRoundInfo(9)
		lastRoundQc := drctypes.NewQuorumCertificate(voteInfo, nil)
		addQCSignature(t, lastRoundQc, "1", s1)
		addQCSignature(t, lastRoundQc, "2", s2)
		addQCSignature(t, lastRoundQc, "3", s3)
		proposeMsg := &ProposalMsg{
			Block: &drctypes.BlockData{
				Author:    "1",
				Round:     10,
				Epoch:     0,
				Timestamp: 1234,
				Payload:   &drctypes.Payload{},
				Qc:        lastRoundQc,
			},
			LastRoundTc: nil,
		}
		require.NoError(t, proposeMsg.Sign(s1))
		return proposeMsg
	}

	require.NoError(t, validProposal(t).Verify(rootTrust))

	t.Run("IsValid is called", func(t *testing.T) {
		prop := validProposal(t)
		prop.Block = nil
		require.EqualError(t, prop.Verify(rootTrust), `validation failed: block is nil`)
	})

	t.Run("unknown signer", func(t *testing.T) {
		prop := validProposal(t)
		prop.Block.Author = "xyz"
		require.EqualError(t, prop.Verify(rootTrust), `signature verification failed: author 'xyz' is not part of the trust base`)
	})

	t.Run("invalid signature", func(t *testing.T) {
		prop := validProposal(t)
		prop.Block.Timestamp = 0x11111111 // changing block after signing makes signature invalid
		require.ErrorContains(t, prop.Verify(rootTrust), `signature verification failed: verify bytes failed`)

		prop.Signature = []byte{0, 1, 2, 3, 4}
		require.ErrorContains(t, prop.Verify(rootTrust), `signature verification failed: verify bytes failed: signature length is 5 b (expected 64 b)`)
	})

	t.Run("no quorum signatures", func(t *testing.T) {
		// this basically tests that Block.Verify is called
		prop := validProposal(t)
		delete(prop.Block.Qc.Signatures, "3")
		require.ErrorContains(t, prop.Verify(rootTrust), `block verification failed: invalid block data QC: failed to verify quorum signatures: quorum not reached, signed_votes=2 quorum_threshold=3`)
	})
}

func TestProposalMsg_Verify_OkWithTc(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := testtb.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3})
	lastRoundVoteInfo := testutils.NewDummyRootRoundInfo(8)
	lastRoundQc := &drctypes.QuorumCert{
		VoteInfo: lastRoundVoteInfo,
		LedgerCommitInfo: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
			seal.PreviousHash = lastRoundVoteInfo.Hash(gocrypto.SHA256)
		}),
		Signatures: map[string][]byte{},
	}
	addQCSignature(t, lastRoundQc, "1", s1)
	addQCSignature(t, lastRoundQc, "2", s2)
	addQCSignature(t, lastRoundQc, "3", s3)
	// round 9 was timeout
	timeout := &drctypes.Timeout{
		Epoch:  0,
		Round:  9,
		HighQc: lastRoundQc,
	}
	lastRoundTc := &drctypes.TimeoutCert{
		Timeout: timeout,
		Signatures: map[string]*drctypes.TimeoutVote{
			"1": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: testutils.CalcTimeoutSig(t, s1, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "1")},
			"2": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: testutils.CalcTimeoutSig(t, s2, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "2")},
			"3": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: testutils.CalcTimeoutSig(t, s3, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "3")},
		},
	}
	proposeMsg := &ProposalMsg{
		Block: &drctypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &drctypes.Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: lastRoundTc,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.NoError(t, proposeMsg.Verify(rootTrust))
}
