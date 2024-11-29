package consensus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/stretchr/testify/require"
)

const testLocalTimeout = time.Duration(10000) * time.Millisecond
const testBlockRate = time.Duration(1000) * time.Millisecond

func TestPacemaker_AdvanceRoundQC(t *testing.T) {
	ctx := context.Background()
	const lastCommittedRound = uint64(6)
	pacemaker, err := NewPacemaker(testBlockRate, testLocalTimeout, observability.Default(t))
	require.NoError(t, err)
	defer pacemaker.Stop()
	pacemaker.Reset(ctx, lastCommittedRound, nil, nil)

	// record vote
	require.Nil(t, pacemaker.GetVoted())
	vote := NewDummyVote(t, "test", 7, []byte{2, 2, 2, 2})
	pacemaker.SetVoted(vote)
	require.Equal(t, vote, pacemaker.GetVoted())

	// nil
	require.False(t, pacemaker.AdvanceRoundQC(ctx, nil))
	require.NotNil(t, pacemaker.GetVoted())

	// old QC
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	staleQc, err := types.NewQuorumCertificate(voteInfo, nil)
	require.NoError(t, err)
	require.False(t, pacemaker.AdvanceRoundQC(ctx, staleQc))
	require.NotNil(t, pacemaker.GetVoted())
	require.EqualValues(t, lastCommittedRound+1, pacemaker.GetCurrentRound())

	// ok QC
	voteInfo = NewDummyVoteInfo(8, []byte{1, 2, 3, 4})
	qc, err := types.NewQuorumCertificate(voteInfo, nil)
	require.NoError(t, err)
	require.True(t, pacemaker.AdvanceRoundQC(ctx, qc))
	require.Equal(t, pacemaker.GetCurrentRound(), uint64(9))
	require.Nil(t, pacemaker.GetVoted(), "expected vote to be reset when view changes")
}

func TestPacemaker_AdvanceRoundTC(t *testing.T) {
	ctx := context.Background()
	const lastCommittedRound = uint64(6)
	pacemaker, err := NewPacemaker(testBlockRate, testLocalTimeout, observability.Default(t))
	require.NoError(t, err)
	defer pacemaker.Stop()
	pacemaker.Reset(ctx, lastCommittedRound, nil, nil)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)

	// record a vote in current round
	vote := NewDummyVote(t, "test", 7, []byte{2, 2, 2, 2})
	pacemaker.SetVoted(vote)
	pacemaker.AdvanceRoundTC(ctx, nil)
	// no change
	require.Equal(t, lastCommittedRound+1, pacemaker.GetCurrentRound())
	require.Equal(t, vote, pacemaker.GetVoted())
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	staleQc, err := types.NewQuorumCertificate(voteInfo, nil)
	require.NoError(t, err)
	staleTc := NewDummyTc(4, staleQc)
	pacemaker.AdvanceRoundTC(ctx, staleTc)
	require.NotNil(t, pacemaker.GetVoted())
	// still no change
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	// create a valid qc for current
	voteInfo = NewDummyVoteInfo(pacemaker.GetCurrentRound()-1, []byte{0, 1, 2, 3})
	qc, err := types.NewQuorumCertificate(voteInfo, nil)
	require.NoError(t, err)
	tc := NewDummyTc(pacemaker.GetCurrentRound(), qc)
	pacemaker.AdvanceRoundTC(ctx, tc)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+2)
	require.Equal(t, tc, pacemaker.LastRoundTC())
	// and vote is reset
	require.Nil(t, pacemaker.GetVoted())
	// Now advance with qc for round 7
	voteInfo = NewDummyVoteInfo(pacemaker.GetCurrentRound(), []byte{0, 1, 2, 3})
	qc, err = types.NewQuorumCertificate(voteInfo, nil)
	require.NoError(t, err)
	require.True(t, pacemaker.AdvanceRoundQC(ctx, qc))
	// now also last round TC is reset
	require.Nil(t, pacemaker.LastRoundTC())
}

func TestPacemaker_RegisterVote(t *testing.T) {
	quorum := NewDummyQuorum(3, 0)

	t.Run("invalid vote - duplicate", func(t *testing.T) {
		pacemaker, err := NewPacemaker(8*time.Second, 10*time.Second, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()
		pacemaker.Reset(context.Background(), 5, nil, nil)
		require.EqualValues(t, 6, pacemaker.GetCurrentRound())

		vote := NewDummyVote(t, "node1", 6, []byte{2, 2, 2, 2})
		qc, mature, err := pacemaker.RegisterVote(vote, quorum)
		require.NoError(t, err)
		require.False(t, mature)
		require.Nil(t, qc)

		qc, mature, err = pacemaker.RegisterVote(vote, quorum)
		require.EqualError(t, err, `vote register error: duplicate vote`)
		require.False(t, mature)
		require.Nil(t, qc)
	})

	t.Run("vote from wrong round", func(t *testing.T) {
		pacemaker, err := NewPacemaker(8*time.Second, 10*time.Second, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()
		pacemaker.Reset(context.Background(), 5, nil, nil)
		require.EqualValues(t, 6, pacemaker.GetCurrentRound())

		vote := NewDummyVote(t, "node1", 5, []byte{2, 2, 2, 2})
		qc, _, err := pacemaker.RegisterVote(vote, quorum)
		require.EqualError(t, err, `received vote is for round 5, current round is 6`)
		require.Nil(t, qc)

		vote = NewDummyVote(t, "node1", 7, []byte{2, 2, 2, 2})
		qc, _, err = pacemaker.RegisterVote(vote, quorum)
		require.EqualError(t, err, `received vote is for round 7, current round is 6`)
		require.Nil(t, qc)
	})

	t.Run("quorum achieved", func(t *testing.T) {
		const lastCommittedRound = uint64(6)
		pacemaker, err := NewPacemaker(8*time.Second, 10*time.Second, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()
		pacemaker.Reset(context.Background(), lastCommittedRound, nil, nil)
		require.Equal(t, lastCommittedRound+1, pacemaker.GetCurrentRound())

		vote := NewDummyVote(t, "node1", 7, []byte{2, 2, 2, 2})
		qc, mature, err := pacemaker.RegisterVote(vote, quorum)
		require.NoError(t, err)
		require.False(t, mature)
		require.Nil(t, qc)

		vote = NewDummyVote(t, "node2", 7, []byte{2, 2, 2, 2})
		qc, mature, err = pacemaker.RegisterVote(vote, quorum)
		require.NoError(t, err)
		require.False(t, mature)
		require.Nil(t, qc)

		vote = NewDummyVote(t, "node3", 7, []byte{2, 2, 2, 2})
		require.NotNil(t, vote)
		qc, mature, err = pacemaker.RegisterVote(vote, quorum)
		require.NoError(t, err)
		require.False(t, mature)
		require.NotNil(t, qc)
	})
}

func TestPacemaker_RegisterTimeoutVote(t *testing.T) {
	const lastCommittedRound = uint64(5)
	quorum := NewDummyQuorum(3, 0)
	pacemaker, err := NewPacemaker(testBlockRate, testLocalTimeout, observability.Default(t))
	require.NoError(t, err)
	defer pacemaker.Stop()
	pacemaker.Reset(context.Background(), lastCommittedRound, nil, nil)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	voteInfo := NewDummyVoteInfo(5, []byte{0, 1, 2, 3})
	hQc, err := types.NewQuorumCertificate(voteInfo, nil)
	require.NoError(t, err)
	vote := NewDummyTimeoutVote(hQc, 6, "node1")
	tc, err := pacemaker.RegisterTimeoutVote(context.Background(), vote, quorum)
	require.NoError(t, err)
	require.Nil(t, tc)
	// node 1 send duplicate
	tc, err = pacemaker.RegisterTimeoutVote(context.Background(), vote, quorum)
	require.EqualError(t, err, "inserting to pending votes: failed to add vote to timeout certificate: node1 already voted in round 6")
	require.Nil(t, tc)
	vote = NewDummyTimeoutVote(hQc, 6, "node2")
	tc, err = pacemaker.RegisterTimeoutVote(context.Background(), vote, quorum)
	require.NoError(t, err)
	require.Nil(t, tc)
	vote = NewDummyTimeoutVote(hQc, 6, "node3")
	tc, err = pacemaker.RegisterTimeoutVote(context.Background(), vote, quorum)
	require.NoError(t, err)
	require.NotNil(t, tc)
}

func TestPacemaker_NewPacemaker(t *testing.T) {
	minRoundLen := 500 * time.Millisecond
	roundTO := time.Second
	pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, pacemaker)
	require.Equal(t, minRoundLen, pacemaker.minRoundLen)
	require.Equal(t, roundTO, pacemaker.maxRoundLen)
	require.EqualValues(t, 0, pacemaker.lastQcToCommitRound)
	require.EqualValues(t, 0, pacemaker.GetCurrentRound())
	require.EqualValues(t, 0, pacemaker.status.Load())
	require.Nil(t, pacemaker.lastRoundTC)
	require.Nil(t, pacemaker.voteSent)
	require.Nil(t, pacemaker.timeoutVote)
	require.Nil(t, pacemaker.currentQC)
	require.NotNil(t, pacemaker.pendingVotes)
	require.NotNil(t, pacemaker.statusChan)
	require.NotNil(t, pacemaker.stopRoundClock)
	require.NotNil(t, pacemaker.tracer)
	require.NotNil(t, pacemaker.roundDur)
	require.NotNil(t, pacemaker.roundCnt)
	// clock shouldn't be ticking until Reset is called
	select {
	case <-time.After(2 * roundTO):
	case e := <-pacemaker.StatusEvents():
		t.Errorf("unexpected event %v", e)
	}
}

func TestPacemaker_startRoundClock(t *testing.T) {
	t.Run("stepping through statuses", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		srcDone := pacemaker.startRoundClock(ctx, minRoundLen, roundTO)
		firstTOevent := time.After(roundTO)
		require.False(t, pacemaker.roundIsMature())

		// there should be no event until minRoundLen has elapsed (we wait a bit less to lessen the timing inaccuracies)
		select {
		case <-time.After(minRoundLen - 20*time.Millisecond):
		case e := <-pacemaker.StatusEvents():
			t.Errorf("unexpected event %v before round matures", e)
		case <-srcDone:
			t.Fatal("unexpectedly roundClock is reported to have been stopped")
		}
		// but we should get pmsRoundMatured before timeout arrives (pretty much instantly now)
		select {
		case <-firstTOevent:
			t.Error("expected to get event before first TO is triggered")
		case <-srcDone:
			t.Fatal("unexpectedly roundClock is reported to have been stopped")
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundMatured {
				t.Errorf("expected event %v got %v", pmsRoundMatured, e)
			}
			require.True(t, pacemaker.roundIsMature())
		}
		// we should get first timeout now - the time it took for the round to mature (minRoundLen) is
		// also part of the first TO so we should get it faster than full "roundTO"
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected event %v got %v", pmsRoundTimeout, e)
			}
			require.True(t, pacemaker.roundIsMature())
		case <-time.After(roundTO):
			t.Errorf("expected to get first TO event before %s elapses", roundTO)
		case <-srcDone:
			t.Fatal("unexpectedly roundClock is reported to have been stopped")
		}
		//...and next TO should arrive after roundTO
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected event %v got %v", pmsRoundTimeout, e)
			}
			require.True(t, pacemaker.roundIsMature())
		case <-time.After(roundTO + 50*time.Millisecond):
			t.Errorf("expected to get second TO event after %s", roundTO)
		case <-srcDone:
			t.Fatal("unexpectedly roundClock is reported to have been stopped")
		}

		// stop the clock
		cancel()
		select {
		case <-srcDone:
			require.False(t, pacemaker.roundIsMature(), "after stopping the clock round should not reported as mature")
			require.EqualValues(t, pmsRoundNone, pacemaker.status.Load())
		case <-time.After(time.Second):
			t.Error("the round clock func hasn't stopped")
		}
	})

	t.Run("cancelling ctx stops next event being sent", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		srcDone := pacemaker.startRoundClock(ctx, minRoundLen, roundTO)

		select {
		case <-time.After(roundTO):
			t.Error("haven't got the expected event")
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundMatured {
				t.Errorf("expected event %v got %v", pmsRoundMatured, e)
			}
		}

		// stop the clock
		cancel()
		select {
		case <-srcDone:
			require.False(t, pacemaker.roundIsMature())
		case <-time.After(time.Second):
			t.Error("the round clock func hasn't stopped")
		}

		// shouldn't get any events now
		select {
		case <-time.After(2 * roundTO):
		case e := <-pacemaker.StatusEvents():
			t.Errorf("unexpected event %v", e)
		}
	})

	t.Run("no leftover ticks", func(t *testing.T) {
		// testing the implementation detail that we use "global ticker" rather than
		// creating new ticker per round - make sure that ticks from previous round
		// do not "leak" into next round triggering event early
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		srcDone := pacemaker.startRoundClock(ctx, minRoundLen, roundTO)

		select {
		case <-time.After(roundTO):
			t.Error("haven't got the expected event")
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundMatured {
				t.Errorf("expected event %v got %v", pmsRoundMatured, e)
			}
		}

		// stop the clock - this stops the round events being created when
		// ticker ticks...
		cancel()
		select {
		case <-srcDone:
		case <-time.After(time.Second):
			t.Error("the round clock func hasn't stopped")
		}
		// ...and wait long enough that ticker should generate new tick
		time.Sleep(roundTO)

		// start new round clock
		ctx, cancel = context.WithCancel(context.Background())
		srcDone = pacemaker.startRoundClock(ctx, minRoundLen, roundTO)
		start := time.Now()

		// we should get pmsRoundMatured but not sooner than minRoundLen
		select {
		case <-time.After(roundTO):
			t.Error("haven't got the expected event")
		case e := <-pacemaker.StatusEvents():
			waited := time.Since(start)
			if e != pmsRoundMatured {
				t.Errorf("expected event %v got %v after %s", pmsRoundMatured, e, waited)
			}
			if waited < minRoundLen {
				t.Errorf("expected that it will take at least %s before receiving event, it took only %s", minRoundLen, waited)
			}
		}

		// stop the clock
		cancel()
		select {
		case <-srcDone:
			require.False(t, pacemaker.roundIsMature())
		case <-time.After(time.Second):
			t.Error("the round clock func hasn't stopped")
		}
	})
}

func TestPacemaker_startNewRound(t *testing.T) {
	t.Run("field values", func(t *testing.T) {
		// test do fields which need to be reset when new round starts do
		// get reset and fields which need to retain their value do
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)
		pacemaker.Reset(context.Background(), 4, nil, nil)
		defer pacemaker.Stop()

		// assign some values to fields, we do not care about validity, we
		// just want to make sure that starting new round sets them nil
		pacemaker.lastRoundTC = &types.TimeoutCert{}
		pacemaker.currentQC = &types.QuorumCert{}
		pacemaker.voteSent = &abdrc.VoteMsg{}
		pacemaker.timeoutVote = &abdrc.TimeoutMsg{}

		pacemaker.startNewRound(context.Background(), 6)
		require.Equal(t, minRoundLen, pacemaker.minRoundLen)
		require.Equal(t, roundTO, pacemaker.maxRoundLen)
		require.EqualValues(t, 4, pacemaker.lastQcToCommitRound)
		require.EqualValues(t, 6, pacemaker.GetCurrentRound())
		require.EqualValues(t, pmsRoundInProgress, pacemaker.status.Load())
		require.Nil(t, pacemaker.voteSent)
		require.Nil(t, pacemaker.timeoutVote)
		require.Nil(t, pacemaker.currentQC)
		require.NotNil(t, pacemaker.lastRoundTC)
		require.NotNil(t, pacemaker.pendingVotes)
		require.NotNil(t, pacemaker.statusChan)
		require.NotNil(t, pacemaker.stopRoundClock)
	})

	t.Run("unread event from previous round is cancelled", func(t *testing.T) {
		// when starting new round (unread) event of the previous round must be
		// cancelled (ie disappear from event chan)
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)
		pacemaker.Reset(context.Background(), 4, nil, nil)
		defer pacemaker.Stop()

		// Reset started new round, wait until timeout without consuming events
		time.Sleep(roundTO + minRoundLen/2)

		// starting new round should cancel the event of the previous round so
		// it should take at least minRoundLen before we get pmsRoundMatured
		start := time.Now()
		pacemaker.startNewRound(context.Background(), 6)
		select {
		case <-time.After(roundTO):
			t.Errorf("expected to get event before %s", roundTO)
		case e := <-pacemaker.StatusEvents():
			waited := time.Since(start)
			if e != pmsRoundMatured {
				t.Errorf("expected event %v got %v after %s", pmsRoundMatured, e, waited)
			}
			if waited < minRoundLen {
				t.Errorf("expected that it will take at least %s before receiving event, it took only %s", minRoundLen, waited)
			}
		}
	})

	t.Run("timeout event is repeated", func(t *testing.T) {
		const TOcycles = 3 // test length, ie how many timeout cycles to count events
		minRoundLen := 200 * time.Millisecond
		roundTO := 600 * time.Millisecond
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)
		pacemaker.Reset(context.Background(), 4, nil, nil)
		defer pacemaker.Stop()

		var timeoutCnt, matureCnt, otherCnt atomic.Uint32
		done := make(chan struct{})
		go func() {
			defer close(done)
			// timers are not exact and there is also some overhead in PM
			// so allow extra 50ms per timeout cycle
			stopCounting := time.After(TOcycles*roundTO + (TOcycles * 75 * time.Millisecond))
			for {
				select {
				case <-stopCounting:
					return
				case e := <-pacemaker.StatusEvents():
					switch e {
					case pmsRoundMatured:
						matureCnt.Add(1)
					case pmsRoundTimeout:
						timeoutCnt.Add(1)
					default:
						otherCnt.Add(1)
					}
				}
			}
		}()

		select {
		case <-time.After((TOcycles + 1) * roundTO):
			t.Error("counting events haven't stopped within timeout")
		case <-done:
		}

		require.Zero(t, otherCnt.Load(), `expected the number of "other" events to be zero`)
		require.EqualValues(t, 1, matureCnt.Load(), "number of %s events", pmsRoundMatured)
		require.EqualValues(t, TOcycles, timeoutCnt.Load(), "number of %s events", pmsRoundTimeout)
	})

	t.Run("timeout vote causes timeout status", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)
		start := time.Now()
		pacemaker.Reset(context.Background(), 4, nil, nil)
		defer pacemaker.Stop()

		// register TO vote with quorum condition which allow no faulty nodes - this means
		// that quorum for the round is not possible anymore and PM should go to TO status.
		// timeout event should be in the event channel right away
		qcRound1, err := types.NewQuorumCertificate(NewDummyVoteInfo(1, []byte{0, 1, 1}), nil)
		require.NoError(t, err)
		timeoutVoteMsg := NewDummyTimeoutVote(qcRound1, 4, "node1")
		tc, err := pacemaker.RegisterTimeoutVote(context.Background(), timeoutVoteMsg, NewDummyQuorum(3, 0))
		require.NoError(t, err)
		require.Nil(t, tc)

		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected to get pmsRoundTimeout event, got %v", e)
			}
		default:
			t.Fatal("expected to get the event immediately")
		}

		// next event must be timeout too but it will happen after minRoundLen as we
		// jumped to TO status without resetting the internal clock
		select {
		case <-time.After(roundTO + roundTO/2):
			t.Fatal("didn't get second event before timeout")
		case e := <-pacemaker.StatusEvents():
			waited := time.Since(start)
			if e != pmsRoundTimeout {
				t.Errorf("expected event %v got %v after %s", pmsRoundTimeout, e, waited)
			}
			if maxDur := minRoundLen + 50*time.Millisecond; minRoundLen > waited || waited > maxDur {
				t.Errorf("expected that it will take between %s and %s to receive event, it took %s", minRoundLen, maxDur, waited)
			}
		}
	})
}

func TestPacemaker_setState(t *testing.T) {
	t.Run("already in pmsRoundTimeout state", func(t *testing.T) {
		roundTO := 500 * time.Millisecond
		pacemaker, err := NewPacemaker(100*time.Millisecond, roundTO, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()

		// "force" status but do not write into event chan (ie event is already consumed)
		pacemaker.status.Store(uint32(pmsRoundTimeout))
		pacemaker.setState(context.Background(), pmsRoundTimeout)
		require.EqualValues(t, pmsRoundTimeout, pacemaker.status.Load())
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected to get pmsRoundTimeout event, got %v", e)
			}
		default:
			t.Error("unexpectedly there is no event in the status event chan")
		}
	})

	t.Run("switching to pmsRoundTimeout state", func(t *testing.T) {
		roundTO := 500 * time.Millisecond
		pacemaker, err := NewPacemaker(100*time.Millisecond, roundTO, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()

		// set current status something other than pmsRoundTimeout
		pacemaker.status.Store(uint32(pmsRoundMatured))
		pacemaker.statusChan <- pmsRoundMatured

		pacemaker.setState(context.Background(), pmsRoundTimeout)
		require.EqualValues(t, pmsRoundTimeout, pacemaker.status.Load())
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected to get pmsRoundTimeout event, got %v", e)
			}
		default:
			t.Error("unexpectedly there is no event in the status event chan")
		}
	})
}

func TestPacemaker_Reset(t *testing.T) {
	t.Run("simulate start with last round TC", func(t *testing.T) {
		// test do fields which need to be reset when new round starts do
		// get reset and fields which need to retain their value do
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()

		tc := &types.TimeoutCert{
			Timeout: &types.Timeout{
				Round: 5,
			},
		}
		pacemaker.Reset(context.Background(), 4, tc, nil)
		require.EqualValues(t, 6, pacemaker.GetCurrentRound())
		require.EqualValues(t, 4, pacemaker.lastQcToCommitRound)
		require.NotNil(t, pacemaker.lastRoundTC)
	})

	t.Run("simulate start with TC from past", func(t *testing.T) {
		// test do fields which need to be reset when new round starts do
		// get reset and fields which need to retain their value do
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()

		tc := &types.TimeoutCert{
			Timeout: &types.Timeout{
				Round: 3,
			},
		}
		pacemaker.Reset(context.Background(), 4, tc, nil)
		require.EqualValues(t, 5, pacemaker.GetCurrentRound())
		require.EqualValues(t, 4, pacemaker.lastQcToCommitRound)
		require.Nil(t, pacemaker.lastRoundTC)
	})

	t.Run("vote message is restored", func(t *testing.T) {
		pacemaker, err := NewPacemaker(time.Second, 500*time.Millisecond, observability.Default(t))
		require.NoError(t, err)
		defer pacemaker.Stop()

		vm := &abdrc.VoteMsg{
			VoteInfo: &types.RoundInfo{RoundNumber: 5},
		}
		pacemaker.Reset(context.Background(), 4, nil, vm)
		require.EqualValues(t, 5, pacemaker.GetCurrentRound())
		require.EqualValues(t, 4, pacemaker.lastQcToCommitRound)
		require.Equal(t, vm, pacemaker.voteSent)
		require.Nil(t, pacemaker.lastRoundTC)
	})

	t.Run("Reset and Stop", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.Default(t))
		require.NoError(t, err)
		require.NotNil(t, pacemaker)

		// assign some values to fields, we do not care about validity, we
		// just want to make sure that Reset sets them nil again
		pacemaker.lastRoundTC = &types.TimeoutCert{}
		pacemaker.currentQC = &types.QuorumCert{}
		pacemaker.voteSent = &abdrc.VoteMsg{}
		pacemaker.timeoutVote = &abdrc.TimeoutMsg{}
		// we haven't called Reset yet so clock should not be ticking
		select {
		case <-time.After(roundTO):
		case e := <-pacemaker.StatusEvents():
			t.Errorf("got unexpected event %v", e)
		}

		// start the clock
		pacemaker.Reset(context.Background(), 10, nil, nil)
		require.EqualValues(t, 10, pacemaker.lastQcToCommitRound)
		require.EqualValues(t, 11, pacemaker.GetCurrentRound())
		require.Nil(t, pacemaker.LastRoundTC())
		require.Nil(t, pacemaker.GetVoted())
		require.Nil(t, pacemaker.GetTimeoutVote())
		require.Nil(t, pacemaker.RoundQC())
		// values Reset should not touch
		require.Equal(t, minRoundLen, pacemaker.minRoundLen)
		require.Equal(t, roundTO, pacemaker.maxRoundLen)
		require.NotNil(t, pacemaker.tracer)
		// we have called Reset so clock must be ticking
		select {
		case <-time.After(roundTO):
			t.Error("unexpectedly haven't got any event")
		case <-pacemaker.StatusEvents():
		}

		// stop the clock
		pacemaker.Stop()
		select {
		case <-time.After(2 * roundTO):
		case e := <-pacemaker.StatusEvents():
			t.Errorf("unexpected event %v after PM stop", e)
		}
	})
}

func TestPacemaker_roundIsMature(t *testing.T) {
	var cases = []struct {
		status paceMakerStatus
		mature bool
	}{
		{status: pmsRoundNone, mature: false},
		{status: pmsRoundInProgress, mature: false},
		{status: pmsRoundMatured, mature: true},
		{status: pmsRoundTimeout, mature: true},
		{status: paceMakerStatus(4), mature: false},
		{status: paceMakerStatus(42), mature: false},
	}

	pacemaker, err := NewPacemaker(time.Second, 2*time.Second, observability.Default(t))
	require.NoError(t, err)

	for x, tc := range cases {
		pacemaker.status.Store(uint32(tc.status))
		if b := pacemaker.roundIsMature(); b != tc.mature {
			t.Errorf("[%d] expected %t got %t", x, tc.mature, b)
		}
	}
}

func Test_paceMakerStatus_String(t *testing.T) {
	var cases = []struct {
		status paceMakerStatus
		str    string
	}{
		{status: pmsRoundNone, str: "pmsRoundNone"},
		{status: pmsRoundInProgress, str: "pmsRoundInProgress"},
		{status: pmsRoundMatured, str: "pmsRoundMatured"},
		{status: pmsRoundTimeout, str: "pmsRoundTimeout"},
		{status: paceMakerStatus(4), str: "unknown status 4"},
		{status: paceMakerStatus(10), str: "unknown status 10"},
	}

	for x, tc := range cases {
		if s := tc.status.String(); s != tc.str {
			t.Errorf("[%d] expected %q got %q", x, tc.str, s)
		}
	}
}
