package distributed

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
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

func NewDummyVoteInfo(round uint64, rootHash []byte) *atomic_broadcast.VoteInfo {
	return &atomic_broadcast.VoteInfo{
		BlockId:       []byte{2},
		RootRound:     round,
		Epoch:         0,
		Timestamp:     1111,
		ParentBlockId: []byte{1},
		ParentRound:   round - 1,
		ExecStateId:   rootHash,
	}
}

func NewDummyQuorumCertificate(voteInfo *atomic_broadcast.VoteInfo, signatures map[string][]byte) *atomic_broadcast.QuorumCert {
	return &atomic_broadcast.QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: NewDummyLedgerCommitInfo(voteInfo),
		Signatures:       signatures,
	}
}

func NewDummyLedgerCommitInfo(voteInfo *atomic_broadcast.VoteInfo) *atomic_broadcast.LedgerCommitInfo {
	return &atomic_broadcast.LedgerCommitInfo{
		VoteInfoHash:  voteInfo.Hash(crypto.SHA256),
		CommitStateId: nil,
	}
}

func NewDummyVote(author string, round uint64, rootHash []byte) *atomic_broadcast.VoteMsg {
	voteInfo := NewDummyVoteInfo(round, rootHash)
	return &atomic_broadcast.VoteMsg{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: NewDummyLedgerCommitInfo(voteInfo),
		HighCommitQc:     nil,
		Author:           author,
		Signature:        []byte{0, 1, 2},
		TimeoutSignature: nil,
	}
}

func NewDummyTimeoutVote(hqc *atomic_broadcast.QuorumCert, vote *atomic_broadcast.VoteMsg) *atomic_broadcast.VoteMsg {
	timeout := vote.NewTimeout(hqc)
	// will not actually sign it, but just create a dummy sig
	dummySig := []byte{0, 1, 2, 3}
	_ = vote.AddTimeoutSignature(timeout, dummySig)
	return vote
}

func TestNewVoteRegister(t *testing.T) {
	voteRegister := NewVoteRegister()
	require.Empty(t, voteRegister.HashToSignatures)
	require.Empty(t, voteRegister.AuthorToVote)
	require.Nil(t, voteRegister.TimeoutCert)
}

func TestVoteRegister_InsertVote(t *testing.T) {
	type args struct {
		vote       []*atomic_broadcast.VoteMsg
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
			args: args{vote: []*atomic_broadcast.VoteMsg{nil},
				quorumInfo: NewDummyQuorum(3)},
			wantQc:  false,
			wantErr: true,
		},
		{
			name: "No quorum",
			args: args{vote: []*atomic_broadcast.VoteMsg{
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
			args: args{vote: []*atomic_broadcast.VoteMsg{
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
			args: args{vote: []*atomic_broadcast.VoteMsg{
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
			var qc *atomic_broadcast.QuorumCert
			var tc *atomic_broadcast.TimeoutCert
			for _, vote := range tt.args.vote {
				qc, tc, err = register.InsertVote(vote, tt.args.quorumInfo)
			}
			if tt.wantQc {
				require.NotNil(t, qc)
			} else {
				require.Nil(t, qc)

			}
			require.Nil(t, tc)
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
	qc, tc, err := register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	// Add vote from node2
	vote = NewDummyVote("node2", 2, []byte{1, 2, 3})
	qc, tc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	// Add vote from node3, but it has different root
	vote = NewDummyVote("node3", 2, []byte{1, 2, 4})
	qc, tc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	// Add vote from node4, but it has different root
	vote = NewDummyVote("node4", 2, []byte{1, 2, 3})
	qc, tc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, tc)
	require.NotNil(t, qc)
	require.Equal(t, qc.VoteInfo.RootRound, uint64(2))
	require.Equal(t, qc.VoteInfo.ParentRound, uint64(1))
	require.Equal(t, qc.VoteInfo.Timestamp, uint64(1111))
	require.Equal(t, qc.VoteInfo.BlockId, []byte{2})
	require.Equal(t, qc.VoteInfo.ParentBlockId, []byte{1})
	require.Equal(t, qc.VoteInfo.ExecStateId, []byte{1, 2, 3})
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
	QcRound1 := NewDummyQuorumCertificate(NewDummyVoteInfo(1, []byte{0, 1, 1}), qcSignatures)
	QcRound2 := NewDummyQuorumCertificate(NewDummyVoteInfo(2, []byte{0, 1, 2}), qcSignatures)
	QcRound3 := NewDummyQuorumCertificate(NewDummyVoteInfo(3, []byte{0, 1, 3}), qcSignatures)

	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	// create dummy timeout vote

	vote := NewDummyVote("node1", 4, []byte{1, 2, 3})
	timeoutVoteMsg := NewDummyTimeoutVote(QcRound1, vote)
	qc, tc, err := register.InsertVote(timeoutVoteMsg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	vote2 := NewDummyVote("node2", 4, []byte{1, 2, 4})
	timeoutVote2Msg := NewDummyTimeoutVote(QcRound2, vote2)
	qc, tc, err = register.InsertVote(timeoutVote2Msg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	vote3 := NewDummyVote("node3", 4, []byte{1, 2, 5})
	timeoutVote3Msg := NewDummyTimeoutVote(QcRound3, vote3)
	qc, tc, err = register.InsertVote(timeoutVote3Msg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	require.NotNil(t, tc)
	require.Equal(t, uint32(len(tc.Signatures)), quorumInfo.GetQuorumThreshold())
	require.Equal(t, tc.Timeout.Round, uint64(4))
	require.Equal(t, tc.Timeout.Epoch, uint64(0))
	// TC must include the most recent QC seen by any node (in this case from QcRound3 - round 3)
	require.Equal(t, tc.Timeout.Hqc.VoteInfo.RootRound, uint64(3))
}

func TestVoteRegister_ErrDuplicateVote(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	qc, tc, err := register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	qc, tc, err = register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.ErrorContains(t, err, "duplicate vote")
}

func TestVoteRegister_ErrEquivocatingVote(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	qc, tc, err := register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	qc, tc, err = register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 4}), quorumInfo)
	require.ErrorContains(t, err, "equivocating vote")
}

func TestVoteRegister_Reset(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3)
	qc, tc, err := register.InsertVote(NewDummyVote("node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	qc, tc, err = register.InsertVote(NewDummyVote("node2", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc, tc)
	register.Reset()
	require.Empty(t, register.HashToSignatures)
	require.Empty(t, register.AuthorToVote)
	require.Nil(t, register.TimeoutCert)
}
