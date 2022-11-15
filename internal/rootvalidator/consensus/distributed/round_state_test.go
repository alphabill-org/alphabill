package distributed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
	const localTimeout = time.Duration(10000) * time.Millisecond
	roundState := NewRoundState(lastCommittedRound, localTimeout)
	require.Equal(t, lastCommittedRound, roundState.highCommittedRound)
	require.Equal(t, lastCommittedRound+1, roundState.currentRound)
	require.Equal(t, time.Now().Add(localTimeout).Round(time.Millisecond), roundState.roundTimeout.Round(time.Millisecond))
	require.Nil(t, roundState.lastRoundTC)
	require.NotNil(t, roundState.pendingVotes)
	require.Nil(t, roundState.voteSent)
}

func TestRoundState_AdvanceRoundQC(t *testing.T) {
	const lastCommittedRound = uint64(6)
	const localTimeout = time.Duration(10000) * time.Millisecond
	roundState := NewRoundState(lastCommittedRound, localTimeout)
	require.Nil(t, roundState.GetVoted())
	// create QC
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	signatures := map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	staleQc := NewDummyQuorumCertificate(voteInfo, signatures)
	vote := NewDummyVote("test", 7, []byte{2, 2, 2, 2})
	// record vote
	roundState.SetVoted(vote)
	require.NotNil(t, roundState.GetVoted())
	// nil
	require.False(t, roundState.AdvanceRoundQC(nil))
	// old QC
	require.False(t, roundState.AdvanceRoundQC(staleQc))
	require.NotNil(t, roundState.GetVoted())
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+1)
	voteInfo = NewDummyVoteInfo(8, []byte{1, 2, 3, 4})
	qc := NewDummyQuorumCertificate(voteInfo, signatures)
	require.True(t, roundState.AdvanceRoundQC(qc))
	require.Equal(t, roundState.GetCurrentRound(), uint64(9))
	// vote is reset when view is changed
	require.Nil(t, roundState.GetVoted())
}

func TestRoundState_AdvanceRoundTC(t *testing.T) {
	const lastCommittedRound = uint64(6)
	const localTimeout = time.Duration(10000) * time.Millisecond
	roundState := NewRoundState(lastCommittedRound, localTimeout)
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+1)
	// record a vote in current round
	vote := NewDummyVote("test", 7, []byte{2, 2, 2, 2})
	// record vote
	roundState.SetVoted(vote)
	roundState.AdvanceRoundTC(nil)
	// no change
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+1)
	require.Equal(t, roundState.GetVoted(), vote)
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	signatures := map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	staleQc := NewDummyQuorumCertificate(voteInfo, signatures)
	staleTc := NewDummyTc(4, staleQc)
	roundState.AdvanceRoundTC(staleTc)
	require.NotNil(t, roundState.GetVoted())
	// still no change
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+1)
	// create a valid qc for current
	voteInfo = NewDummyVoteInfo(roundState.GetCurrentRound()-1, []byte{0, 1, 2, 3})
	qc := NewDummyQuorumCertificate(voteInfo, signatures)
	tc := NewDummyTc(roundState.GetCurrentRound(), qc)
	roundState.AdvanceRoundTC(tc)
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+2)
	require.Equal(t, tc, roundState.LastRoundTC())
	// and vote is reset
	require.Nil(t, roundState.GetVoted())
	// Now advance with qc for round 7
	voteInfo = NewDummyVoteInfo(roundState.GetCurrentRound(), []byte{0, 1, 2, 3})
	qc = NewDummyQuorumCertificate(voteInfo, signatures)
	require.True(t, roundState.AdvanceRoundQC(qc))
	// now also last round TC is reset
	require.Nil(t, roundState.LastRoundTC())
}

func TestRoundState_GetRoundTimeout(t *testing.T) {
	const lastCommittedRound = uint64(6)
	const localTimeout = time.Duration(10000) * time.Millisecond
	roundState := NewRoundState(lastCommittedRound, localTimeout)
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+1)
	// round to ms, a few ns have passed, but the timeout should be still round localTimeout
	roundTimeout := roundState.GetRoundTimeout().Round(time.Millisecond)
	require.Equal(t, localTimeout, roundTimeout)
	voteInfo := NewDummyVoteInfo(roundState.GetCurrentRound()-1, []byte{0, 1, 2, 3})
	signatures := map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	qc := NewDummyQuorumCertificate(voteInfo, signatures)
	tc := NewDummyTc(roundState.GetCurrentRound(), qc)
	roundState.AdvanceRoundTC(tc)
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+2)
	roundTimeout = roundState.GetRoundTimeout().Round(time.Millisecond)
	require.Equal(t, localTimeout, roundTimeout)
}

func TestRoundState_RegisterVote(t *testing.T) {
	const lastCommittedRound = uint64(6)
	const localTimeout = time.Duration(10000) * time.Millisecond
	quorum := NewDummyQuorum(3)
	roundState := NewRoundState(lastCommittedRound, localTimeout)
	require.Equal(t, roundState.GetCurrentRound(), lastCommittedRound+1)
	vote := NewDummyVote("node1", 7, []byte{2, 2, 2, 2})
	qc, tc := roundState.RegisterVote(vote, quorum)
	require.Nil(t, qc, tc)
	vote = NewDummyVote("node2", 7, []byte{2, 2, 2, 2})
	qc, tc = roundState.RegisterVote(vote, quorum)
	require.Nil(t, qc, tc)
	vote = NewDummyVote("node3", 7, []byte{2, 2, 2, 2})
	qc, tc = roundState.RegisterVote(vote, quorum)
	require.Nil(t, tc)
	require.NotNil(t, qc)
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
