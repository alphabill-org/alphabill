package ab_consensus

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func (x *QuorumCert) addSignatureToQc(t *testing.T, author string, signer crypto.Signer) {
	sig, err := signer.SignBytes(x.LedgerCommitInfo.Bytes())
	require.NoError(t, err)
	x.Signatures[author] = sig
}

func TestProposalMsg_IsValid(t *testing.T) {
	type fields struct {
		Block  *BlockData
		LastTc *TimeoutCert
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
			wantErrStr: "proposal msg not valid, block is nil",
		},
		{
			name: "Block not valid",
			fields: fields{
				Block: &BlockData{
					Author:    "",
					Round:     0,
					Epoch:     0,
					Timestamp: 0,
					Payload:   nil,
					Qc:        nil,
				},
			},
			wantErrStr: "proposal msg not valid, block error: invalid round number",
		},
		{
			name: "Invalid block round, does not follow QC",
			fields: fields{
				Block: &BlockData{
					Author:    "test",
					Round:     9,
					Epoch:     0,
					Timestamp: 1670314583525,
					Payload:   &Payload{},
					Qc: &QuorumCert{
						VoteInfo:         NewDummyVoteInfo(7),
						LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(7)),
						Signatures:       map[string][]byte{"test": {0, 1, 2, 3}},
					},
				},
			},
			wantErrStr: "proposal round 9 does not follow certified round 7",
		},
		{
			name: "Invalid block round, does not follow QC nor TC",
			fields: fields{
				Block: &BlockData{
					Author:    "test",
					Round:     9,
					Epoch:     0,
					Timestamp: 1670314583525,
					Payload:   &Payload{},
					Qc: &QuorumCert{
						VoteInfo:         NewDummyVoteInfo(5),
						LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(5)),
						Signatures:       map[string][]byte{"test": {0, 1, 2, 3}},
					},
				},
				LastTc: &TimeoutCert{
					Timeout: &Timeout{
						Epoch:  0,
						Round:  7,
						HighQc: nil,
					},
				},
			},
			wantErrStr: "proposal round 9 does not follow certified round 7",
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
	prevRoundInfo := NewDummyVoteInfo(8)
	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     9,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc: &QuorumCert{
				VoteInfo:         prevRoundInfo,
				LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: prevRoundInfo.Hash(gocrypto.SHA256)},
				Signatures:       map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	require.ErrorIs(t, errSignerIsNil, proposeMsg.Sign(nil))
}

func TestProposalMsg_Sign_InvalidBlock(t *testing.T) {
	qcInfo := NewDummyVoteInfo(8)

	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     0,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc: &QuorumCert{
				VoteInfo:         qcInfo,
				LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: qcInfo.Hash(gocrypto.SHA256)},
				Signatures:       map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	require.ErrorContains(t, proposeMsg.Sign(s1), "invalid round")
}

func TestProposalMsg_Sign_Ok(t *testing.T) {
	qcInfo := NewDummyVoteInfo(8)

	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc: &QuorumCert{
				VoteInfo:         qcInfo,
				LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: qcInfo.Hash(gocrypto.SHA256)},
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
	lastRoundQc := NewQuorumCertificate(NewDummyVoteInfo(9), nil)
	lastRoundQc.addSignatureToQc(t, "1", s1)
	lastRoundQc.addSignatureToQc(t, "2", s2)
	lastRoundQc.addSignatureToQc(t, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "12",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "failed to find public key for root validator 12")
}

func TestProposalMsg_Verify_ErrorInBlockHash(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := NewDummyVoteInfo(9)
	lastRoundQc := &QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, lastRoundVoteInfo),
		Signatures:       map[string][]byte{},
	}
	lastRoundQc.addSignatureToQc(t, "1", s1)
	lastRoundQc.addSignatureToQc(t, "2", s2)
	lastRoundQc.addSignatureToQc(t, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	// change block after signing
	proposeMsg.Block.Timestamp = 0x11111111
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "proposal msg signature verification error")
}

func TestProposalMsg_Verify_BlockQcNoQuorum(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	_, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := NewDummyVoteInfo(9)
	lastRoundQc := &QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	lastRoundQc.addSignatureToQc(t, "1", s1)
	lastRoundQc.addSignatureToQc(t, "2", s2)
	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "proposal msg block verification failed")
}

func TestProposalMsg_Verify_InvalidSignature(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := NewDummyVoteInfo(9)
	lastRoundQc := &QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	lastRoundQc.addSignatureToQc(t, "1", s1)
	lastRoundQc.addSignatureToQc(t, "2", s2)
	lastRoundQc.addSignatureToQc(t, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: nil,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	proposeMsg.Signature = []byte{0, 1, 2, 3, 4}
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "proposal msg signature verification error")
}

func TestProposalMsg_Verify_OK(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := NewDummyVoteInfo(9)
	lastRoundQc := &QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	lastRoundQc.addSignatureToQc(t, "1", s1)
	lastRoundQc.addSignatureToQc(t, "2", s2)
	lastRoundQc.addSignatureToQc(t, "3", s3)
	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
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
	lastRoundVoteInfo := NewDummyVoteInfo(8)
	lastRoundQc := &QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: lastRoundVoteInfo.Hash(gocrypto.SHA256)},
		Signatures:       map[string][]byte{},
	}
	lastRoundQc.addSignatureToQc(t, "1", s1)
	lastRoundQc.addSignatureToQc(t, "2", s2)
	lastRoundQc.addSignatureToQc(t, "3", s3)
	// round 9 was timeout
	timeout := &Timeout{
		Epoch:  0,
		Round:  9,
		HighQc: lastRoundQc,
	}
	lastRoundTc := &TimeoutCert{
		Timeout: timeout,
		Signatures: map[string]*TimeoutVote{
			"1": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: calcTimeoutSig(t, s1, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "1")},
			"2": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: calcTimeoutSig(t, s2, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "2")},
			"3": {HqcRound: lastRoundVoteInfo.RoundNumber, Signature: calcTimeoutSig(t, s3, timeout.Round, timeout.Epoch, lastRoundVoteInfo.RoundNumber, "3")},
		},
	}
	proposeMsg := &ProposalMsg{
		Block: &BlockData{
			Author:    "1",
			Round:     10,
			Epoch:     0,
			Timestamp: 1234,
			Payload:   &Payload{},
			Qc:        lastRoundQc,
		},
		LastRoundTc: lastRoundTc,
	}
	require.NoError(t, proposeMsg.Sign(s1))
	require.NoError(t, proposeMsg.Verify(3, rootTrust))
}
