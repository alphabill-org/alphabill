package distributed

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/stretchr/testify/require"
)

const testLocalTimeout = time.Duration(10000) * time.Millisecond
const testBlockRate = time.Duration(1000) * time.Millisecond

func TestExponentialTimeInterval_GetNextTimeout(t *testing.T) {
	type fields struct {
		baseMs       time.Duration
		exponentBase float64
		maxExponent  uint8
	}
	type args struct {
		roundsAfterLastCommit uint64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []time.Duration
	}{
		{
			name: "Get constant timeout",
			fields: fields{
				baseMs:       time.Duration(500) * time.Millisecond,
				exponentBase: 1.2,
				maxExponent:  0,
			},
			want: []time.Duration{
				time.Duration(500) * time.Millisecond,
				time.Duration(500) * time.Millisecond,
				time.Duration(500) * time.Millisecond,
			},
		},
		{
			name: "Get exponential backoff timeout",
			fields: fields{
				baseMs:       time.Duration(500) * time.Millisecond,
				exponentBase: 1.2,
				maxExponent:  3,
			},
			want: []time.Duration{
				time.Duration(500) * time.Millisecond,
				time.Duration(600) * time.Millisecond,
				time.Duration(720) * time.Millisecond,
				time.Duration(864) * time.Millisecond,
				time.Duration(864) * time.Millisecond,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := ExponentialTimeInterval{
				baseMs:       tt.fields.baseMs,
				exponentBase: tt.fields.exponentBase,
				maxExponent:  tt.fields.maxExponent,
			}
			for i, duration := range tt.want {
				if got := x.GetNextTimeout(uint64(i)); got != duration {
					t.Errorf("%v GetNextTimeout() = %v, want %v", i, got, duration)
				}
			}
		})
	}
}

func TestNewRoundState(t *testing.T) {
	const lastCommittedRound = uint64(2)
	pacemaker := NewPacemaker(lastCommittedRound, testLocalTimeout, testBlockRate)
	require.Equal(t, time.Now().Add(testLocalTimeout).Round(time.Millisecond), pacemaker.roundTimeout.Round(time.Millisecond))
	require.Equal(t, lastCommittedRound, pacemaker.lastQcToCommitRound)
	require.Equal(t, lastCommittedRound+1, pacemaker.currentRound)
	require.Nil(t, pacemaker.lastRoundTC)
	require.NotNil(t, pacemaker.pendingVotes)
	require.Nil(t, pacemaker.voteSent)
}

func TestRoundState_AdvanceRoundQC(t *testing.T) {
	const lastCommittedRound = uint64(6)
	pacemaker := NewPacemaker(lastCommittedRound, testLocalTimeout, testBlockRate)
	require.Nil(t, pacemaker.GetVoted())
	// create QC
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	staleQc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	vote := NewDummyVote("test", 7, []byte{2, 2, 2, 2})
	// record vote
	pacemaker.SetVoted(vote)
	require.NotNil(t, pacemaker.GetVoted())
	// nil
	require.False(t, pacemaker.AdvanceRoundQC(nil))
	// old QC
	require.False(t, pacemaker.AdvanceRoundQC(staleQc))
	require.NotNil(t, pacemaker.GetVoted())
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	voteInfo = NewDummyVoteInfo(8, []byte{1, 2, 3, 4})
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	require.True(t, pacemaker.AdvanceRoundQC(qc))
	require.Equal(t, pacemaker.GetCurrentRound(), uint64(9))
	// vote is reset when view is changed
	require.Nil(t, pacemaker.GetVoted())
}

func TestRoundState_AdvanceRoundTC(t *testing.T) {
	const lastCommittedRound = uint64(6)
	pacemaker := NewPacemaker(lastCommittedRound, testLocalTimeout, testBlockRate)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	// record a vote in current round
	vote := NewDummyVote("test", 7, []byte{2, 2, 2, 2})
	// record vote
	pacemaker.SetVoted(vote)
	pacemaker.AdvanceRoundTC(nil)
	// no change
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	require.Equal(t, pacemaker.GetVoted(), vote)
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	staleQc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	staleTc := NewDummyTc(4, staleQc)
	pacemaker.AdvanceRoundTC(staleTc)
	require.NotNil(t, pacemaker.GetVoted())
	// still no change
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	// create a valid qc for current
	voteInfo = NewDummyVoteInfo(pacemaker.GetCurrentRound()-1, []byte{0, 1, 2, 3})
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	tc := NewDummyTc(pacemaker.GetCurrentRound(), qc)
	pacemaker.AdvanceRoundTC(tc)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+2)
	require.Equal(t, tc, pacemaker.LastRoundTC())
	// and vote is reset
	require.Nil(t, pacemaker.GetVoted())
	// Now advance with qc for round 7
	voteInfo = NewDummyVoteInfo(pacemaker.GetCurrentRound(), []byte{0, 1, 2, 3})
	qc = atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	require.True(t, pacemaker.AdvanceRoundQC(qc))
	// now also last round TC is reset
	require.Nil(t, pacemaker.LastRoundTC())
}

func TestRoundState_GetRoundTimeout(t *testing.T) {
	const lastCommittedRound = uint64(6)
	const localTimeout = time.Duration(10000) * time.Millisecond
	pacemaker := NewPacemaker(lastCommittedRound, testLocalTimeout, testBlockRate)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	// round to ms, a few ns have passed, but the timeout should be still round localTimeout
	roundTimeout := pacemaker.GetRoundTimeout().Round(time.Millisecond)
	require.Equal(t, localTimeout, roundTimeout)
	voteInfo := NewDummyVoteInfo(pacemaker.GetCurrentRound()-1, []byte{0, 1, 2, 3})
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	tc := NewDummyTc(pacemaker.GetCurrentRound(), qc)
	pacemaker.AdvanceRoundTC(tc)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+2)
	roundTimeout = pacemaker.GetRoundTimeout().Round(time.Millisecond)
	require.Equal(t, localTimeout, roundTimeout)
}

func TestRoundState_RegisterVote(t *testing.T) {
	const lastCommittedRound = uint64(6)
	quorum := NewDummyQuorum(3)
	pacemaker := NewPacemaker(lastCommittedRound, testLocalTimeout, testBlockRate)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	vote := NewDummyVote("node1", 7, []byte{2, 2, 2, 2})
	qc := pacemaker.RegisterVote(vote, quorum)
	require.Nil(t, qc)
	vote = NewDummyVote("node2", 7, []byte{2, 2, 2, 2})
	qc = pacemaker.RegisterVote(vote, quorum)
	require.Nil(t, qc)
	vote = NewDummyVote("node3", 7, []byte{2, 2, 2, 2})
	qc = pacemaker.RegisterVote(vote, quorum)
	require.NotNil(t, qc)
}

func TestRoundState_OddRoundCalcTimeTilFirstProposal(t *testing.T) {
	const lastCommittedRound = uint64(0)
	pacemaker := NewPacemaker(lastCommittedRound, testLocalTimeout, testBlockRate)
	timeout := pacemaker.CalcTimeTilNextProposal()
	// subtract some small amount of time to reduce race
	require.Greater(t, timeout, testBlockRate-(time.Duration(5)*time.Millisecond))
}

func TestRoundState_EvenRoundCalcTimeTilFirstProposal(t *testing.T) {
	const lastCommittedRound = uint64(1)
	pacemaker := NewPacemaker(lastCommittedRound, testLocalTimeout, testBlockRate)
	timeout := pacemaker.CalcTimeTilNextProposal()
	require.Equal(t, timeout, time.Duration(0))
}

func Test_min(t *testing.T) {
	type args struct {
		x uint64
		y uint64
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{
			name: "min is x",
			args: args{x: 1, y: 2},
			want: 1,
		},
		{
			name: "min is y",
			args: args{x: 2, y: 1},
			want: 1,
		},
		{
			name: "equal",
			args: args{x: 1, y: 1},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := min(tt.args.x, tt.args.y); got != tt.want {
				t.Errorf("min() = %v, want %v", got, tt.want)
			}
		})
	}
}