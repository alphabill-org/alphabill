package distributed

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
	"github.com/stretchr/testify/require"
)

type DummyQuorum struct {
	quorum uint32
}

func NewDummyQuorum(q uint32) *DummyQuorum {
	return &DummyQuorum{quorum: q}
}
func (d *DummyQuorum) GetQuorumThreshold() uint32 {
	return d.quorum
}

func NewDummyVoteInfo(round uint64, rootHash []byte) *certificates.RootRoundInfo {
	return &certificates.RootRoundInfo{
		RoundNumber:       round,
		Epoch:             0,
		Timestamp:         1111,
		ParentRoundNumber: round - 1,
		CurrentRootHash:   rootHash,
	}
}

func NewDummyLedgerCommitInfo(voteInfo *certificates.RootRoundInfo) *certificates.CommitInfo {
	return &certificates.CommitInfo{
		RootRoundInfoHash: voteInfo.Hash(gocrypto.SHA256),
		RootHash:          nil,
	}
}

func NewDummyVote(author string, round uint64, rootHash []byte) *ab_consensus.VoteMsg {
	voteInfo := NewDummyVoteInfo(round, rootHash)
	return &ab_consensus.VoteMsg{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: NewDummyLedgerCommitInfo(voteInfo),
		HighQc:           nil,
		Author:           author,
		Signature:        []byte{0, 1, 2},
	}
}

func NewDummyTimeoutVote(hqc *ab_consensus.QuorumCert, round uint64, author string) *ab_consensus.TimeoutMsg {
	timeoutMsg := ab_consensus.NewTimeoutMsg(
		ab_consensus.NewTimeout(round, 0, hqc), author)
	// will not actually sign it, but just create a dummy sig
	timeoutMsg.Signature = []byte{0, 1, 2, 3}
	return timeoutMsg
}

func NewDummyTc(round uint64, qc *ab_consensus.QuorumCert) *ab_consensus.TimeoutCert {
	timeout := &ab_consensus.Timeout{
		Epoch:  0,
		Round:  round,
		HighQc: qc,
	}
	// will not actually sign it, but just create a dummy sig
	dummySig := []byte{0, 1, 2, 3}
	sigs := map[string]*ab_consensus.TimeoutVote{
		"node1": {HqcRound: 2, Signature: dummySig},
		"node2": {HqcRound: 2, Signature: dummySig},
		"node3": {HqcRound: 2, Signature: dummySig},
	}
	return &ab_consensus.TimeoutCert{
		Timeout:    timeout,
		Signatures: sigs,
	}
}

func TestNewVoteRegister(t *testing.T) {
	voteRegister := NewVoteRegister()
	require.Empty(t, voteRegister.hashToSignatures)
	require.Empty(t, voteRegister.authorToVote)
	require.Nil(t, voteRegister.timeoutCert)
}

func TestVoteRegister_InsertVote(t *testing.T) {
	type args struct {
		vote       []*ab_consensus.VoteMsg
		quorumInfo QuorumInfo
	}
	tests := []struct {
		name    string
		args    args
		wantQc  bool
		wantErr bool
	}{
		{
			name: "Add nil vote",
			args: args{vote: []*ab_consensus.VoteMsg{nil},
				quorumInfo: NewDummyQuorum(3)},
			wantQc:  false,
			wantErr: true,
		},
		{
			name: "No quorum",
			args: args{vote: []*ab_consensus.VoteMsg{
				NewDummyVote("node1", 2, []byte{1, 2, 3}),
				NewDummyVote("node2", 2, []byte{1, 2, 3}),
				NewDummyVote("node3", 2, []byte{1, 2, 4}),
			},
				quorumInfo: NewDummyQuorum(3)},
			wantQc:  false,
			wantErr: false,
		},
		{
			name: "Quorum ok",
			args: args{vote: []*ab_consensus.VoteMsg{
				NewDummyVote("node1", 2, []byte{1, 2, 3}),
				NewDummyVote("node2", 2, []byte{1, 2, 3}),
				NewDummyVote("node3", 2, []byte{1, 2, 3}),
			},
				quorumInfo: NewDummyQuorum(3)},
			wantQc:  true,
			wantErr: false,
		},
		{
			name: "Quorum 5 ok",
			args: args{vote: []*ab_consensus.VoteMsg{
				NewDummyVote("node1", 2, []byte{1, 2, 3}),
				NewDummyVote("node2", 2, []byte{1, 2, 3}),
				NewDummyVote("node3", 2, []byte{1, 2, 3}),
				NewDummyVote("node4", 2, []byte{1, 2, 4}),
				NewDummyVote("node5", 2, []byte{1, 2, 3}),
				NewDummyVote("node6", 2, []byte{1, 2, 4}),
				NewDummyVote("node7", 2, []byte{1, 2, 4}),
				NewDummyVote("node8", 2, []byte{1, 2, 3}),
			},
				quorumInfo: NewDummyQuorum(5)},
			wantQc:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			register := NewVoteRegister()
			var err error
			var qc *ab_consensus.QuorumCert
			for _, vote := range tt.args.vote {
				qc, err = register.InsertVote(vote, tt.args.quorumInfo)
			}
			if tt.wantQc {
				require.NotNil(t, qc)
			} else {
				require.Nil(t, qc)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("InsertVote() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestVoteRegister_Qc(t *testing.T) {
	quorumInfo := NewDummyQuorum(3)
	register := NewVoteRegister()
	// Add vote from node1
	vote := NewDummyVote("node1", 2, []byte{1, 2, 3})
	qc, err := register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	// Add vote from node2
	vote = NewDummyVote("node2", 2, []byte{1, 2, 3})
	qc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	// Add vote from node3, but it has different root
	vote = NewDummyVote("node3", 2, []byte{1, 2, 4})
	qc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	// Add vote from node4, but it has different root
	vote = NewDummyVote("node4", 2, []byte{1, 2, 3})
	qc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.NotNil(t, qc)
	require.Equal(t, qc.VoteInfo.RoundNumber, uint64(2))
	require.Equal(t, qc.VoteInfo.ParentRoundNumber, uint64(1))
	require.Equal(t, qc.VoteInfo.Timestamp, uint64(1111))
	require.Equal(t, qc.VoteInfo.CurrentRootHash, []byte{1, 2, 3})
	require.Equal(t, qc.LedgerCommitInfo, vote.LedgerCommitInfo)
	require.Contains(t, qc.Signatures, "node1")
	require.Contains(t, qc.Signatures, "node2")
	require.Contains(t, qc.Signatures, "node4")
	require.NotContains(t, qc.Signatures, "node3")

}

func TestVoteRegister_Tc(t *testing.T) {
	qcSignatures := map[string][]byte{
		"node1": {0, 1, 2, 3},
		"node2": {0, 1, 2, 3},
		"node3": {0, 1, 2, 3},
	}
	qcRound1 := ab_consensus.NewQuorumCertificate(NewDummyVoteInfo(1, []byte{0, 1, 1}), nil)
	qcRound1.Signatures = qcSignatures
	qcRound2 := ab_consensus.NewQuorumCertificate(NewDummyVoteInfo(2, []byte{0, 1, 2}), nil)
	qcRound2.Signatures = qcSignatures
	qcRound3 := ab_consensus.NewQuorumCertificate(NewDummyVoteInfo(3, []byte{0, 1, 3}), nil)
	qcRound3.Signatures = qcSignatures

	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	// create dummy timeout vote

	timeoutVoteMsg := NewDummyTimeoutVote(qcRound1, 4, "node1")
	tc, err := register.InsertTimeoutVote(timeoutVoteMsg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, tc)
	timeoutVote2Msg := NewDummyTimeoutVote(qcRound2, 4, "node2")
	tc, err = register.InsertTimeoutVote(timeoutVote2Msg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, tc)
	timeoutVote3Msg := NewDummyTimeoutVote(qcRound3, 4, "node3")
	tc, err = register.InsertTimeoutVote(timeoutVote3Msg, quorumInfo)
	require.NoError(t, err)
	require.NotNil(t, tc)
	require.Equal(t, uint32(len(tc.Signatures)), quorumInfo.GetQuorumThreshold())
	require.Equal(t, tc.Timeout.Round, uint64(4))
	require.Equal(t, tc.Timeout.Epoch, uint64(0))
	// TC must include the most recent QC seen by any node (in this case from qcRound3 - round 3)
	require.Equal(t, tc.Timeout.HighQc.VoteInfo.RoundNumber, uint64(3))
}

func TestVoteRegister_ErrDuplicateVote(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	qc, err := register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	qc, err = register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.ErrorContains(t, err, "duplicate vote")
}

func TestVoteRegister_ErrEquivocatingVote(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	qc, err := register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	qc, err = register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 4}), quorumInfo)
	require.ErrorContains(t, err, "equivocating vote")
}

func TestVoteRegister_Reset(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	qcSignatures := map[string][]byte{
		"node1": {0, 1, 2, 3},
		"node2": {0, 1, 2, 3},
		"node3": {0, 1, 2, 3},
	}
	qcRound1 := ab_consensus.NewQuorumCertificate(NewDummyVoteInfo(1, []byte{0, 1, 1}), nil)
	qcRound1.Signatures = qcSignatures
	qc, err := register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	qc, err = register.InsertVote(NewDummyVote("node2", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	timeoutVoteMsg := NewDummyTimeoutVote(qcRound1, 4, "test")
	tc, err := register.InsertTimeoutVote(timeoutVoteMsg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, tc)
	register.Reset()
	require.Empty(t, register.hashToSignatures)
	require.Empty(t, register.authorToVote)
	require.Nil(t, register.timeoutCert)
}
