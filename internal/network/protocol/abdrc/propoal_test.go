package abdrc

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/testutils"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func addQCSignature(t *testing.T, qc *abtypes.QuorumCert, author string, signer crypto.Signer) {
	t.Helper()
	sig, err := signer.SignBytes(qc.LedgerCommitInfo.Bytes())
	require.NoError(t, err)
	qc.Signatures[author] = sig
}

func TestProposalMsg_IsValid(t *testing.T) {
	type fields struct {
		Block  *abtypes.BlockData
		LastTc *abtypes.TimeoutCert
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
				Block: &abtypes.BlockData{
					Author:    "",
					Round:     0,
					Epoch:     0,
					Timestamp: 0,
					Payload:   nil,
					Qc:        nil,
				},
			},
			wantErrStr: "block not valid: invalid round number",
		},
		{
			name: "Invalid block round, does not follow QC",
			fields: fields{
				Block: &abtypes.BlockData{
					Author:    "test",
					Round:     9,
					Epoch:     0,
					Timestamp: 1670314583525,
					Payload:   &abtypes.Payload{},
					Qc: &abtypes.QuorumCert{
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
				Block: &abtypes.BlockData{
					Author:    "test",
					Round:     9,
					Epoch:     0,
					Timestamp: 1670314583525,
					Payload:   &abtypes.Payload{},
					Qc: &abtypes.QuorumCert{
						VoteInfo:         testutils.NewDummyRootRoundInfo(5),
						LedgerCommitInfo: testutils.NewDummyCommitInfo(gocrypto.SHA256, testutils.NewDummyRootRoundInfo(5)),
						Signatures:       map[string][]byte{"test": {0, 1, 2, 3}},
					},
				},
				LastTc: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{
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
	prevRoundInfo := &abtypes.RoundInfo{RoundNumber: 8}
	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     9,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc: &abtypes.QuorumCert{
				VoteInfo:         prevRoundInfo,
				LedgerCommitInfo: &abtypes.CommitInfo{RootRoundInfoHash: prevRoundInfo.Hash(gocrypto.SHA256)},
				Signatures:       map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	require.ErrorIs(t, errSignerIsNil, proposeMsg.Sign(nil))
}

func TestProposalMsg_Sign_InvalidBlock(t *testing.T) {
	qcInfo := &abtypes.RoundInfo{RoundNumber: 8}

	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     0,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc: &abtypes.QuorumCert{
				VoteInfo:         qcInfo,
				LedgerCommitInfo: &abtypes.CommitInfo{RootRoundInfoHash: qcInfo.Hash(gocrypto.SHA256)},
				Signatures:       map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	require.ErrorContains(t, proposeMsg.Sign(s1), "invalid round")
}

func TestProposalMsg_Sign_Ok(t *testing.T) {
	qcInfo := testutils.NewDummyRootRoundInfo(8)

	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc: &abtypes.QuorumCert{
				VoteInfo:         qcInfo,
				LedgerCommitInfo: &abtypes.CommitInfo{RootRoundInfoHash: qcInfo.Hash(gocrypto.SHA256)},
				Signatures:       map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	require.NoError(t, proposeMsg.Sign(s1))
}

func TestProposalMsg_Verify_UnknownSigner(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	voteInfo := testutils.NewDummyRootRoundInfo(9)
	lastRoundQc := abtypes.NewQuorumCertificate(voteInfo, nil)
	addQCSignature(t, lastRoundQc, "1", s1)
	addQCSignature(t, lastRoundQc, "2", s2)
	addQCSignature(t, lastRoundQc, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "12",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "unknown root validator 12")
}

func TestProposalMsg_Verify_ErrorInBlockHash(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := testutils.NewDummyRootRoundInfo(9)
	lastRoundQc := &abtypes.QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: testutils.NewDummyCommitInfo(gocrypto.SHA256, lastRoundVoteInfo),
		Signatures:       map[string][]byte{},
	}
	addQCSignature(t, lastRoundQc, "1", s1)
	addQCSignature(t, lastRoundQc, "2", s2)
	addQCSignature(t, lastRoundQc, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	// change block after signing
	proposeMsg.Block.Timestamp = 0x11111111
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "message signature verification failed")
}

func TestProposalMsg_Verify_BlockQcNoQuorum(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	_, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := testutils.NewDummyRootRoundInfo(9)
	lastRoundQc := &abtypes.QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &abtypes.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	addQCSignature(t, lastRoundQc, "1", s1)
	addQCSignature(t, lastRoundQc, "2", s2)
	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "block verification failed")
}

func TestProposalMsg_Verify_InvalidSignature(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := testutils.NewDummyRootRoundInfo(9)
	lastRoundQc := &abtypes.QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &abtypes.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	addQCSignature(t, lastRoundQc, "1", s1)
	addQCSignature(t, lastRoundQc, "2", s2)
	addQCSignature(t, lastRoundQc, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	proposeMsg.Signature = []byte{0, 1, 2, 3, 4}
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "signature verification failed")
}

func TestProposalMsg_Verify_OK(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := testutils.NewDummyRootRoundInfo(9)
	lastRoundQc := &abtypes.QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &abtypes.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	addQCSignature(t, lastRoundQc, "1", s1)
	addQCSignature(t, lastRoundQc, "2", s2)
	addQCSignature(t, lastRoundQc, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.NoError(t, proposeMsg.Verify(3, rootTrust))
}

func TestProposalMsg_Verify_OkWithTc(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := testutils.NewDummyRootRoundInfo(8)
	lastRoundQc := &abtypes.QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &abtypes.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	addQCSignature(t, lastRoundQc, "1", s1)
	addQCSignature(t, lastRoundQc, "2", s2)
	addQCSignature(t, lastRoundQc, "3", s3)
	// round 9 was timeout
	timeout := &abtypes.Timeout{
		Epoch:  0,
		Round:  9,
		HighQc: lastRoundQc,
	}
	lastRoundTc := &abtypes.TimeoutCert{
		Timeout: timeout,
		Signatures: map[string]*abtypes.TimeoutVote{
			"1": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: testutils.CalcTimeoutSig(t, s1, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "1")},
			"2": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: testutils.CalcTimeoutSig(t, s2, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "2")},
			"3": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: testutils.CalcTimeoutSig(t, s3, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "3")},
		},
	}
	proposeMsg := &ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &abtypes.Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: lastRoundTc,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.NoError(t, proposeMsg.Verify(3, rootTrust))
}
