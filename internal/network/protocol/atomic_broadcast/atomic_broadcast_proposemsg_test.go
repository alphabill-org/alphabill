package atomic_broadcast

import (
	gocrypto "crypto"
	"testing"

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
		Block *BlockData
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
					Id:        nil,
					Author:    "",
					Round:     0,
					Epoch:     0,
					Timestamp: 0,
					Payload:   nil,
					Qc:        nil,
				},
			},
			wantErrStr: "proposal msg not valid, block error: invalid block id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &ProposalMsg{
				Block:       tt.fields.Block,
				LastRoundTc: nil, // timeout certificate is optional
				Signature:   nil, // IsValid does not check signatures
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
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, prevRoundInfo),
				Signatures:       map[string][]byte{"1": {1, 2, 3}},
			},
		},
		LastRoundTc: nil,
	}
	require.ErrorIs(t, ErrSignerIsNil, proposeMsg.Sign(nil))
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
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, qcInfo),
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
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, qcInfo),
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

func TestProposalMsg_Verify_BlockHashDoesNotMatchId(t *testing.T) {
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
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "proposal msg error, bock hash does not match block id")
}

func TestProposalMsg_Verify_BlockQcNoQuorum(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	_, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	lastRoundVoteInfo := NewDummyVoteInfo(9)
	lastRoundQc := &QuorumCert{
		VoteInfo:         lastRoundVoteInfo,
		LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, lastRoundVoteInfo),
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
	proposeMsg.Signature = []byte{0, 1, 2, 3, 4}
	require.ErrorContains(t, proposeMsg.Verify(3, rootTrust), "proposal msg signature verification failed")
}

func TestProposalMsg_Verify_OK(t *testing.T) {
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
		LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, lastRoundVoteInfo),
		Signatures:       map[string][]byte{},
	}
	lastRoundQc.addSignatureToQc(t, "1", s1)
	lastRoundQc.addSignatureToQc(t, "2", s2)
	lastRoundQc.addSignatureToQc(t, "3", s3)
	// round 9 was timeout
	timeout := &Timeout{
		Epoch: 0,
		Round: 9,
		Hqc:   lastRoundQc,
	}
	timeoutMsg1 := NewTimeoutSign(0, 9, lastRoundVoteInfo.RootRound)
	tMsgSig1, err := s1.SignHash(timeoutMsg1.Hash(gocrypto.SHA256))
	require.NoError(t, err)
	timeoutMsg2 := NewTimeoutSign(0, 9, lastRoundVoteInfo.RootRound)
	tMsgSig2, err := s2.SignHash(timeoutMsg2.Hash(gocrypto.SHA256))
	require.NoError(t, err)
	timeoutMsg3 := NewTimeoutSign(0, 9, lastRoundVoteInfo.RootRound)
	tMsgSig3, err := s3.SignHash(timeoutMsg3.Hash(gocrypto.SHA256))
	require.NoError(t, err)
	lastRoundTc := &TimeoutCert{
		Timeout: timeout,
		Signatures: map[string]*TimeoutVote{
			"1": {HqcRound: timeoutMsg1.hqcRound, Signature: tMsgSig1},
			"2": {HqcRound: timeoutMsg2.hqcRound, Signature: tMsgSig2},
			"3": {HqcRound: timeoutMsg3.hqcRound, Signature: tMsgSig3},
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
