package distributed

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
)

func initSafetyModule(t *testing.T) *SafetyModule {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	safety, err := NewSafetyModule(signer)
	require.NoError(t, err)
	require.NotNil(t, safety)
	require.NotNil(t, safety.verifier)
	return safety
}

func TestIsConsecutive(t *testing.T) {
	const currentRound = 4
	// block is deemed consecutive if it follows current round 4 i.e. block with round 5 is consecutive
	require.False(t, isConsecutive(4, currentRound))
	require.True(t, isConsecutive(5, currentRound))
	require.False(t, isConsecutive(6, currentRound))
}

func TestNewSafetyModule(t *testing.T) {
	safety := initSafetyModule(t)
	require.Equal(t, uint64(0), safety.highestQcRound)
	require.Equal(t, uint64(0), safety.highestVotedRound)
}

func TestSafetyModule_IsSafeToVote(t *testing.T) {
	var blockRound uint64 = 4
	s := initSafetyModule(t)
	// Assume last round was not TC - TC is nil
	// then it is safe to vote:
	// 1. must vote in monotonically increasing rounds
	// 2. must extend a smaller round
	var tc *atomic_broadcast.TimeoutCert = nil
	require.False(t, s.isSafeToVote(blockRound, 2, tc))
	require.True(t, s.isSafeToVote(blockRound, 3, tc))
	require.False(t, s.isSafeToVote(blockRound, 4, tc))
	// create dummy TC for last round
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	signatures := map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	qc := NewDummyQuorumCertificate(voteInfo, signatures)
	tc = NewDummyTc(4, qc)
	// Is safe because block follows directly to QC
	require.True(t, s.isSafeToVote(blockRound, 3, tc))
	// Not safe because block round does not follow TC round
	require.False(t, s.isSafeToVote(blockRound, 2, tc))
	tc = NewDummyTc(3, qc)
	// Not safe because TC highest QC round seen is bigger than QC round
	require.False(t, s.isSafeToVote(blockRound, 2, tc))
	// Safe, block follows TC and the QC round is >= TC higest QC round seen by the group
	voteInfo = NewDummyVoteInfo(1, []byte{0, 1, 2, 3})
	qc = NewDummyQuorumCertificate(voteInfo, signatures)
	tc = NewDummyTc(3, qc)
	require.True(t, s.isSafeToVote(blockRound, 2, tc))
}

func TestSafetyModule_MakeVote(t *testing.T) {
	s := initSafetyModule(t)
	dummyRootHash := []byte{1, 2, 3}
	blockData := &atomic_broadcast.BlockData{
		Id:        []byte{0, 1, 2},
		Author:    "test",
		Round:     4,
		Epoch:     0,
		Timestamp: 10000,
		Payload:   nil,
		Qc:        nil,
	}
	var tc *atomic_broadcast.TimeoutCert = nil
	vote, err := s.MakeVote(blockData, dummyRootHash, "node1", tc)
	require.Error(t, atomic_broadcast.ErrMissingQuorumCertificate, err)
	require.Nil(t, vote)
	// try to make a successful dummy vote
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	signatures := map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	// create a dummy QC
	blockData.Qc = NewDummyQuorumCertificate(voteInfo, signatures)
	vote, err = s.MakeVote(blockData, dummyRootHash, "node1", tc)
	require.NoError(t, err)
	require.NotNil(t, vote)
	require.Equal(t, "node1", vote.Author)
	require.Greater(t, len(vote.Signature), 1)
	require.NotNil(t, vote.LedgerCommitInfo)
	require.Equal(t, uint64(3), s.highestQcRound)
	require.Equal(t, uint64(4), s.highestVotedRound)
	// try to sign the same vote again
	vote, err = s.MakeVote(blockData, dummyRootHash, "node1", tc)
	// only allowed to vote for monotonically increasing rounds
	require.ErrorContains(t, err, "Not safe to vote")
	require.Nil(t, vote)

}

func TestSafetyModule_SignProposal(t *testing.T) {
	s := initSafetyModule(t)
	// create a dummy proposal message
	proposal := &atomic_broadcast.ProposalMsg{
		Block: &atomic_broadcast.BlockData{
			Id:        []byte{0, 1, 2},
			Author:    "test",
			Round:     4,
			Epoch:     0,
			Timestamp: 10000,
			Payload:   nil,
			Qc:        nil,
		},
		HighCommitQc: nil,
		LastRoundTc:  nil,
	}
	// invalid block missing payload and QC
	require.ErrorIs(t, s.SignProposal(proposal), atomic_broadcast.ErrMissingPayload)
	// add empty payload
	proposal.Block.Payload = &atomic_broadcast.Payload{Requests: nil}
	// still missing QC
	require.ErrorIs(t, s.SignProposal(proposal), atomic_broadcast.ErrMissingQuorumCertificate)
	// create dummy QC
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	signatures := map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	qc := NewDummyQuorumCertificate(voteInfo, signatures)
	proposal.Block.Qc = qc
	require.NoError(t, s.SignProposal(proposal))
	require.Greater(t, len(proposal.Signature), 1)
}

func TestSafetyModule_SignTimeout(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	s := &SafetyModule{
		highestVotedRound: 3,
		highestQcRound:    2,
		signer:            signer,
	}
	require.NotNil(t, s)
	// previous round did not timeout
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	signatures := map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	qc := NewDummyQuorumCertificate(voteInfo, signatures)
	timeout := &atomic_broadcast.Timeout{
		Epoch: 0,
		Round: 3,
		Hqc:   qc,
	}
	sig, err := s.SignTimeout(timeout, nil)
	require.ErrorContains(t, err, "not safe to timeout")
	require.Nil(t, sig)
	timeout.Round = 4
	sig, err = s.SignTimeout(timeout, nil)
	require.NoError(t, err)
	require.NotNil(t, sig)
}

func TestSafetyModule_constructLedgerCommitInfo(t *testing.T) {
	type fields struct {
		highestVotedRound uint64
		highestQcRound    uint64
		signer            crypto.Signer
	}
	type args struct {
		block        *atomic_broadcast.BlockData
		voteInfoHash []byte
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *atomic_broadcast.LedgerCommitInfo
	}{
		{
			name:   "To be committed",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 2, ParentRound: 1, ExecStateId: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: &atomic_broadcast.LedgerCommitInfo{VoteInfoHash: []byte{2, 2, 2, 2}, CommitStateId: []byte{0, 1, 2, 3}},
		},
		{
			name:   "Not to be committed",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 1, ParentRound: 0, ExecStateId: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: &atomic_broadcast.LedgerCommitInfo{VoteInfoHash: []byte{2, 2, 2, 2}, CommitStateId: nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SafetyModule{
				highestVotedRound: tt.fields.highestVotedRound,
				highestQcRound:    tt.fields.highestQcRound,
				signer:            tt.fields.signer,
			}
			if got := s.constructLedgerCommitInfo(tt.args.block, tt.args.voteInfoHash); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("constructLedgerCommitInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafetyModule_isCommitCandidate(t *testing.T) {
	type fields struct {
		highestVotedRound uint64
		highestQcRound    uint64
		signer            crypto.Signer
	}
	type args struct {
		block *atomic_broadcast.BlockData
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []byte
	}{
		{
			name:   "Is candidate",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 2, ExecStateId: []byte{0, 1, 2, 3}},
				},
			}},
			want: []byte{0, 1, 2, 3},
		},
		{
			name:   "Not candidate, block round does not follow QC round",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 1, ExecStateId: []byte{0, 1, 2, 3}},
				},
			}},
			want: nil,
		},
		{
			name:   "Not candidate, QC is nil",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc:    nil,
			}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SafetyModule{
				highestVotedRound: tt.fields.highestVotedRound,
				highestQcRound:    tt.fields.highestQcRound,
				signer:            tt.fields.signer,
			}
			if got := s.isCommitCandidate(tt.args.block); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("isCommitCandidate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafetyModule_isSafeToTimeout(t *testing.T) {
	type fields struct {
		highestVotedRound uint64
		highestQcRound    uint64
		signer            crypto.Signer
	}
	type args struct {
		round   uint64                        // timeout round
		qcRound uint64                        // timeout highest seen QC round
		tc      *atomic_broadcast.TimeoutCert // previous round TC
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name:   "OK",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{round: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						Hqc: &atomic_broadcast.QuorumCert{
							VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 1},
						}}}},
			want: true,
		},
		{
			name:   "Not safe to timeout, last round was not TC, but QC round is smaller than the QC we have seen",
			fields: fields{highestVotedRound: 2, highestQcRound: 2, signer: nil},
			args:   args{round: 2, qcRound: 1, tc: nil},
			want:   false,
		},
		{
			name:   "Already voted for round 2 and can vote again for timeout",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args:   args{round: 2, qcRound: 1, tc: nil},
			want:   true,
		},
		{
			name:   "round does not follow QC or TC",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{round: 2, qcRound: 2,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 3,
						Hqc: &atomic_broadcast.QuorumCert{
							VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 1},
						}}}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SafetyModule{
				highestVotedRound: tt.fields.highestVotedRound,
				highestQcRound:    tt.fields.highestQcRound,
				signer:            tt.fields.signer,
			}
			if got := s.isSafeToTimeout(tt.args.round, tt.args.qcRound, tt.args.tc); got != tt.want {
				t.Errorf("isSafeToTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isSaveToExtend(t *testing.T) {
	type args struct {
		blockRound uint64
		qcRound    uint64
		tc         *atomic_broadcast.TimeoutCert
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Safe to extend from TC, block follows TC",
			args: args{blockRound: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 1,
						Hqc: &atomic_broadcast.QuorumCert{
							VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 1},
						}}}},
			want: true,
		},
		{
			name: "Not safe to extend from TC, block does not follow TC",
			args: args{blockRound: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						Hqc: &atomic_broadcast.QuorumCert{
							VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 1},
						}}}},
			want: false,
		},
		{
			name: "Not safe to extend from TC, block does not follow TC",
			args: args{blockRound: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 3,
						Hqc: &atomic_broadcast.QuorumCert{
							VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 1},
						}}}},
			want: false,
		},
		{
			name: "Not safe to extend from TC, block follows TC, but hqc round is higher than qc round",
			args: args{blockRound: 3, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						Hqc: &atomic_broadcast.QuorumCert{
							VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 3},
						}}}},
			want: false,
		},
		{
			name: "Safe to extend from TC, block follows TC and hqc round is equal to qc round",
			args: args{blockRound: 3, qcRound: 3,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						Hqc: &atomic_broadcast.QuorumCert{
							VoteInfo: &atomic_broadcast.VoteInfo{RootRound: 3},
						}}}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeToExtend(tt.args.blockRound, tt.args.qcRound, tt.args.tc); got != tt.want {
				t.Errorf("isSafeToExtend() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_max(t *testing.T) {
	type args struct {
		a uint64
		b uint64
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{
			name: "a is max",
			args: args{a: 1, b: 0},
			want: 1,
		},
		{
			name: "b is max",
			args: args{a: 0, b: 2},
			want: 2,
		},
		{
			name: "equal",
			args: args{a: 3, b: 3},
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := max(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("max() = %v, want %v", got, tt.want)
			}
		})
	}
}
