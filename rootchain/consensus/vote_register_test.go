package consensus

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/stretchr/testify/require"
)

type DummyQuorum struct {
	quorum uint64
	faulty uint64
}

func NewDummyQuorum(q, f uint64) *DummyQuorum {
	return &DummyQuorum{quorum: q, faulty: f}
}
func (d *DummyQuorum) GetQuorumThreshold() uint64 {
	return d.quorum
}
func (d *DummyQuorum) GetMaxFaultyNodes() uint64 { return d.faulty }

func NewDummyVoteInfo(round uint64, rootHash []byte) *drctypes.RoundInfo {
	return &drctypes.RoundInfo{
		RoundNumber:       round,
		Epoch:             0,
		Timestamp:         1111,
		ParentRoundNumber: round - 1,
		CurrentRootHash:   rootHash,
	}
}

func NewDummyLedgerCommitInfo(t *testing.T, voteInfo *drctypes.RoundInfo) *types.UnicitySeal {
	h, err := voteInfo.Hash(gocrypto.SHA256)
	require.NoError(t, err)
	return &types.UnicitySeal{
		Version:      1,
		PreviousHash: h,
		Hash:         nil,
	}
}

func NewDummyVote(t *testing.T, author string, round uint64, rootHash []byte) *abdrc.VoteMsg {
	voteInfo := NewDummyVoteInfo(round, rootHash)
	return &abdrc.VoteMsg{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: NewDummyLedgerCommitInfo(t, voteInfo),
		HighQc:           nil,
		Author:           author,
		Signature:        []byte{0, 1, 2},
	}
}

func NewDummyTimeoutVote(hqc *drctypes.QuorumCert, round uint64, author string) *abdrc.TimeoutMsg {
	timeoutMsg := abdrc.NewTimeoutMsg(drctypes.NewTimeout(round, 0, hqc), author, nil)
	// will not actually sign it, but just create a dummy sig
	timeoutMsg.Signature = []byte{0, 1, 2, 3}
	return timeoutMsg
}

func NewDummyTc(round uint64, qc *drctypes.QuorumCert) *drctypes.TimeoutCert {
	timeout := &drctypes.Timeout{
		Epoch:  0,
		Round:  round,
		HighQc: qc,
	}
	// will not actually sign it, but just create a dummy sig
	dummySig := []byte{0, 1, 2, 3}
	sigs := map[string]*drctypes.TimeoutVote{
		"node1": {HqcRound: 2, Signature: dummySig},
		"node2": {HqcRound: 2, Signature: dummySig},
		"node3": {HqcRound: 2, Signature: dummySig},
	}
	return &drctypes.TimeoutCert{
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
		vote       []*abdrc.VoteMsg
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
			args: args{vote: []*abdrc.VoteMsg{nil},
				quorumInfo: NewDummyQuorum(3, 0)},
			wantQc:  false,
			wantErr: true,
		},
		{
			name: "No quorum",
			args: args{vote: []*abdrc.VoteMsg{
				NewDummyVote(t, "node1", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node2", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node3", 2, []byte{1, 2, 4}),
			},
				quorumInfo: NewDummyQuorum(3, 0)},
			wantQc:  false,
			wantErr: false,
		},
		{
			name: "Quorum ok",
			args: args{vote: []*abdrc.VoteMsg{
				NewDummyVote(t, "node1", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node2", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node3", 2, []byte{1, 2, 3}),
			},
				quorumInfo: NewDummyQuorum(3, 0)},
			wantQc:  true,
			wantErr: false,
		},
		{
			name: "Quorum 5 ok",
			args: args{vote: []*abdrc.VoteMsg{
				NewDummyVote(t, "node1", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node2", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node3", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node4", 2, []byte{1, 2, 4}),
				NewDummyVote(t, "node5", 2, []byte{1, 2, 3}),
				NewDummyVote(t, "node6", 2, []byte{1, 2, 4}),
				NewDummyVote(t, "node7", 2, []byte{1, 2, 4}),
				NewDummyVote(t, "node8", 2, []byte{1, 2, 3}),
			},
				quorumInfo: NewDummyQuorum(5, 0)},
			wantQc:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			register := NewVoteRegister()
			var err error
			var qc *drctypes.QuorumCert
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
	quorumInfo := NewDummyQuorum(3, 0)
	register := NewVoteRegister()
	// Add vote from node1
	vote := NewDummyVote(t, "node1", 2, []byte{1, 2, 3})
	qc, err := register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	// Add vote from node2
	vote = NewDummyVote(t, "node2", 2, []byte{1, 2, 3})
	qc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	// Add vote from node3, but it has different root
	vote = NewDummyVote(t, "node3", 2, []byte{1, 2, 4})
	qc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	// Add vote from node4, but it has different root
	vote = NewDummyVote(t, "node4", 2, []byte{1, 2, 3})
	qc, err = register.InsertVote(vote, quorumInfo)
	require.NoError(t, err)
	require.NotNil(t, qc)
	require.Equal(t, qc.VoteInfo.RoundNumber, uint64(2))
	require.Equal(t, qc.VoteInfo.ParentRoundNumber, uint64(1))
	require.Equal(t, qc.VoteInfo.Timestamp, uint64(1111))
	require.Equal(t, qc.VoteInfo.CurrentRootHash, hex.Bytes{1, 2, 3})
	require.Equal(t, qc.LedgerCommitInfo, vote.LedgerCommitInfo)
	require.Contains(t, qc.Signatures, "node1")
	require.Contains(t, qc.Signatures, "node2")
	require.Contains(t, qc.Signatures, "node4")
	require.NotContains(t, qc.Signatures, "node3")

}

func TestVoteRegister_Tc(t *testing.T) {
	qcSignatures := map[string]hex.Bytes{
		"node1": {0, 1, 2, 3},
		"node2": {0, 1, 2, 3},
		"node3": {0, 1, 2, 3},
	}
	qcRound1, err := drctypes.NewQuorumCertificate(NewDummyVoteInfo(1, []byte{0, 1, 1}), nil)
	require.NoError(t, err)
	qcRound1.Signatures = qcSignatures
	qcRound2, err := drctypes.NewQuorumCertificate(NewDummyVoteInfo(2, []byte{0, 1, 2}), nil)
	require.NoError(t, err)
	qcRound2.Signatures = qcSignatures
	qcRound3, err := drctypes.NewQuorumCertificate(NewDummyVoteInfo(3, []byte{0, 1, 3}), nil)
	require.NoError(t, err)
	qcRound3.Signatures = qcSignatures

	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3, 0)
	// create dummy timeout vote

	timeoutVoteMsg := NewDummyTimeoutVote(qcRound1, 4, "node1")
	tc, voteCnt, err := register.InsertTimeoutVote(timeoutVoteMsg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, tc)
	require.EqualValues(t, 1, voteCnt)
	timeoutVote2Msg := NewDummyTimeoutVote(qcRound2, 4, "node2")
	tc, voteCnt, err = register.InsertTimeoutVote(timeoutVote2Msg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, tc)
	require.EqualValues(t, 2, voteCnt)
	// attempt to add vote1 again should fail
	tc, voteCnt, err = register.InsertTimeoutVote(timeoutVoteMsg, quorumInfo)
	require.EqualError(t, err, `failed to add vote to timeout certificate: node1 already voted in round 4`)
	require.Nil(t, tc)
	require.Zero(t, voteCnt)
	// adding another unique vote should get us quorum
	timeoutVote3Msg := NewDummyTimeoutVote(qcRound3, 4, "node3")
	tc, voteCnt, err = register.InsertTimeoutVote(timeoutVote3Msg, quorumInfo)
	require.NoError(t, err)
	require.NotNil(t, tc)
	require.EqualValues(t, 3, voteCnt)
	require.Equal(t, uint64(len(tc.Signatures)), quorumInfo.GetQuorumThreshold())
	require.Equal(t, tc.Timeout.Round, uint64(4))
	require.Equal(t, tc.Timeout.Epoch, uint64(0))
	// TC must include the most recent QC seen by any node (in this case from qcRound3 - round 3)
	require.Equal(t, tc.Timeout.HighQc.VoteInfo.RoundNumber, uint64(3))
}

func TestVoteRegister_ErrDuplicateVote(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3, 0)
	qc, err := register.InsertVote(NewDummyVote(t, "node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	qc, err = register.InsertVote(NewDummyVote(t, "node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.ErrorContains(t, err, "duplicate vote")
	require.Nil(t, qc)
}

func TestVoteRegister_ErrEquivocatingVote(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3, 0)
	qc, err := register.InsertVote(NewDummyVote(t, "node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	qc, err = register.InsertVote(NewDummyVote(t, "node1", 2, []byte{1, 2, 4}), quorumInfo)
	require.ErrorContains(t, err, "equivocating vote")
	require.Nil(t, qc)
}

func TestVoteRegister_Reset(t *testing.T) {
	register := NewVoteRegister()
	quorumInfo := NewDummyQuorum(3, 0)
	qcSignatures := map[string]hex.Bytes{
		"node1": {0, 1, 2, 3},
		"node2": {0, 1, 2, 3},
		"node3": {0, 1, 2, 3},
	}
	qcRound1, err := drctypes.NewQuorumCertificate(NewDummyVoteInfo(1, []byte{0, 1, 1}), nil)
	require.NoError(t, err)
	qcRound1.Signatures = qcSignatures
	qc, err := register.InsertVote(NewDummyVote(t, "node1", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	qc, err = register.InsertVote(NewDummyVote(t, "node2", 2, []byte{1, 2, 3}), quorumInfo)
	require.NoError(t, err)
	require.Nil(t, qc)
	timeoutVoteMsg := NewDummyTimeoutVote(qcRound1, 4, "test")
	tc, voteCnt, err := register.InsertTimeoutVote(timeoutVoteMsg, quorumInfo)
	require.NoError(t, err)
	require.Nil(t, tc)
	require.EqualValues(t, 1, voteCnt)

	register.Reset()
	require.Empty(t, register.hashToSignatures)
	require.Empty(t, register.authorToVote)
	require.Nil(t, register.timeoutCert)
}
