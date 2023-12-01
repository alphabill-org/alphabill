package abdrc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
)

const testLocalTimeout = time.Duration(10000) * time.Millisecond
const testBlockRate = time.Duration(1000) * time.Millisecond

func TestRoundState_AdvanceRoundQC(t *testing.T) {
	const lastCommittedRound = uint64(6)
	pacemaker, err := NewPacemaker(testBlockRate, testLocalTimeout, observability.NOPMetrics())
	require.NoError(t, err)
	defer pacemaker.Stop()
	pacemaker.Reset(lastCommittedRound, nil, nil)

	// record vote
	require.Nil(t, pacemaker.GetVoted())
	vote := NewDummyVote("test", 7, []byte{2, 2, 2, 2})
	pacemaker.SetVoted(vote)
	require.Equal(t, vote, pacemaker.GetVoted())

	// nil
	require.False(t, pacemaker.AdvanceRoundQC(nil))
	require.NotNil(t, pacemaker.GetVoted())

	// old QC
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	staleQc := abtypes.NewQuorumCertificate(voteInfo, nil)
	require.False(t, pacemaker.AdvanceRoundQC(staleQc))
	require.NotNil(t, pacemaker.GetVoted())
	require.EqualValues(t, lastCommittedRound+1, pacemaker.GetCurrentRound())

	// ok QC
	voteInfo = NewDummyVoteInfo(8, []byte{1, 2, 3, 4})
	qc := abtypes.NewQuorumCertificate(voteInfo, nil)
	require.True(t, pacemaker.AdvanceRoundQC(qc))
	require.Equal(t, pacemaker.GetCurrentRound(), uint64(9))
	require.Nil(t, pacemaker.GetVoted(), "expected vote to be reset when view changes")
}

func TestRoundState_AdvanceRoundTC(t *testing.T) {
	const lastCommittedRound = uint64(6)
	pacemaker, err := NewPacemaker(testBlockRate, testLocalTimeout, observability.NOPMetrics())
	require.NoError(t, err)
	defer pacemaker.Stop()
	pacemaker.Reset(lastCommittedRound, nil, nil)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)

	// record a vote in current round
	vote := NewDummyVote("test", 7, []byte{2, 2, 2, 2})
	pacemaker.SetVoted(vote)
	pacemaker.AdvanceRoundTC(nil)
	// no change
	require.Equal(t, lastCommittedRound+1, pacemaker.GetCurrentRound())
	require.Equal(t, vote, pacemaker.GetVoted())
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	staleQc := abtypes.NewQuorumCertificate(voteInfo, nil)
	staleTc := NewDummyTc(4, staleQc)
	pacemaker.AdvanceRoundTC(staleTc)
	require.NotNil(t, pacemaker.GetVoted())
	// still no change
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	// create a valid qc for current
	voteInfo = NewDummyVoteInfo(pacemaker.GetCurrentRound()-1, []byte{0, 1, 2, 3})
	qc := abtypes.NewQuorumCertificate(voteInfo, nil)
	tc := NewDummyTc(pacemaker.GetCurrentRound(), qc)
	pacemaker.AdvanceRoundTC(tc)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+2)
	require.Equal(t, tc, pacemaker.LastRoundTC())
	// and vote is reset
	require.Nil(t, pacemaker.GetVoted())
	// Now advance with qc for round 7
	voteInfo = NewDummyVoteInfo(pacemaker.GetCurrentRound(), []byte{0, 1, 2, 3})
	qc = abtypes.NewQuorumCertificate(voteInfo, nil)
	require.True(t, pacemaker.AdvanceRoundQC(qc))
	// now also last round TC is reset
	require.Nil(t, pacemaker.LastRoundTC())
}

func TestRoundState_RegisterVote(t *testing.T) {
	const lastCommittedRound = uint64(6)
	quorum := NewDummyQuorum(3, 0)
	pacemaker, err := NewPacemaker(testBlockRate, testLocalTimeout, observability.NOPMetrics())
	require.NoError(t, err)
	defer pacemaker.Stop()
	pacemaker.Reset(lastCommittedRound, nil, nil)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	vote := NewDummyVote("node1", 7, []byte{2, 2, 2, 2})
	qc, _, err := pacemaker.RegisterVote(vote, quorum)
	require.NoError(t, err)
	require.Nil(t, qc)
	vote = NewDummyVote("node2", 7, []byte{2, 2, 2, 2})
	qc, _, err = pacemaker.RegisterVote(vote, quorum)
	require.NoError(t, err)
	require.Nil(t, qc)
	vote = NewDummyVote("node3", 7, []byte{2, 2, 2, 2})
	require.NotNil(t, vote)
	qc, _, err = pacemaker.RegisterVote(vote, quorum)
	require.NoError(t, err)
	require.NotNil(t, qc)
}

func TestRoundState_RegisterTimeoutVote(t *testing.T) {
	const lastCommittedRound = uint64(5)
	quorum := NewDummyQuorum(3, 0)
	pacemaker, err := NewPacemaker(testBlockRate, testLocalTimeout, observability.NOPMetrics())
	require.NoError(t, err)
	defer pacemaker.Stop()
	pacemaker.Reset(lastCommittedRound, nil, nil)
	require.Equal(t, pacemaker.GetCurrentRound(), lastCommittedRound+1)
	voteInfo := NewDummyVoteInfo(5, []byte{0, 1, 2, 3})
	hQc := abtypes.NewQuorumCertificate(voteInfo, nil)
	vote := NewDummyTimeoutVote(hQc, 6, "node1")
	tc, err := pacemaker.RegisterTimeoutVote(vote, quorum)
	require.NoError(t, err)
	require.Nil(t, tc)
	// node 1 send duplicate
	tc, err = pacemaker.RegisterTimeoutVote(vote, quorum)
	require.ErrorContains(t, err, "inserting to pending votes: failed to add vote to timeout certificate: node1 already voted")
	require.Nil(t, tc)
	vote = NewDummyTimeoutVote(hQc, 6, "node2")
	tc, err = pacemaker.RegisterTimeoutVote(vote, quorum)
	require.NoError(t, err)
	require.Nil(t, tc)
	vote = NewDummyTimeoutVote(hQc, 6, "node3")
	tc, err = pacemaker.RegisterTimeoutVote(vote, quorum)
	require.NoError(t, err)
	require.NotNil(t, tc)
}

func TestPacemaker_setup(t *testing.T) {
	t.Run("state of new instance", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
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
		require.NotNil(t, pacemaker.ticker)
		// ticker shouldn't be ticking
		select {
		case <-time.After(2 * roundTO):
		case <-pacemaker.ticker.C:
			t.Error("unexpected tick")
		case e := <-pacemaker.StatusEvents():
			t.Errorf("unexpected event %v", e)
		}
	})

	t.Run("Reset and Stop", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		require.NotNil(t, pacemaker)

		// assign some values to fields, we do not care about validity, we
		// just want to make sure that Reset sets them nil again
		pacemaker.lastRoundTC = &abtypes.TimeoutCert{}
		pacemaker.currentQC = &abtypes.QuorumCert{}
		pacemaker.voteSent = &abdrc.VoteMsg{}
		pacemaker.timeoutVote = &abdrc.TimeoutMsg{}
		// we haven't called Reset yet so clock should not be ticking
		select {
		case <-time.After(roundTO):
		case <-pacemaker.ticker.C:
			t.Error("unexpected tick")
		case e := <-pacemaker.StatusEvents():
			t.Errorf("got unexpected event %v", e)
		}

		// start the clock
		pacemaker.Reset(10, nil, nil)
		require.EqualValues(t, 10, pacemaker.lastQcToCommitRound)
		require.EqualValues(t, 11, pacemaker.GetCurrentRound())
		require.Nil(t, pacemaker.LastRoundTC())
		require.Nil(t, pacemaker.GetVoted())
		require.Nil(t, pacemaker.GetTimeoutVote())
		require.Nil(t, pacemaker.RoundQC())
		// values Reset should not touch
		require.Equal(t, minRoundLen, pacemaker.minRoundLen)
		require.Equal(t, roundTO, pacemaker.maxRoundLen)
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
		case <-pacemaker.ticker.C:
			t.Error("unexpected tick")
		case e := <-pacemaker.StatusEvents():
			t.Errorf("unexpected event %v", e)
		}
	})
}

func TestPacemaker_startRoundClock(t *testing.T) {
	t.Run("stepping through statuses", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		srcDone := pacemaker.startRoundClock(ctx, minRoundLen, roundTO)
		firstTOevent := time.After(roundTO)

		// there should be no event until minRoundLen has elapsed (we wait a bit less to lessen the timing inaccuracies)
		select {
		case <-time.After(minRoundLen - 20*time.Millisecond):
		case e := <-pacemaker.StatusEvents():
			t.Errorf("unexpected event %v before round matures", e)
		}
		// but we should get pmsRoundMatured before timeout arrives (pretty much instantly now)
		select {
		case <-firstTOevent:
			t.Error("expected to get event before first TO is triggered")
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundMatured {
				t.Errorf("expected event %v got %v", pmsRoundMatured, e)
			}
		}
		// we should get first timeout now - the time it took for the round to mature (minRoundLen) is
		// also part of the first TO so we should get it faster than full "roundTO"
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected event %v got %v", pmsRoundTimeout, e)
			}
		case <-time.After(roundTO):
			t.Errorf("expected to get first TO event before %s elapses", roundTO)
		}
		//...and next TO should arrive after roundTO
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected event %v got %v", pmsRoundTimeout, e)
			}
		case <-time.After(roundTO + 50*time.Millisecond):
			t.Errorf("expected to get second TO event after %s", roundTO)
		}

		// stop the clock
		cancel()
		select {
		case <-srcDone:
		case <-time.After(time.Second):
			t.Error("the round clock func hasn't stopped")
		}
	})

	t.Run("cancelling ctx stops next event being sent", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
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
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
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
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		pacemaker.Reset(4, nil, nil)
		defer pacemaker.Stop()

		// assign some values to fields, we do not care about validity, we
		// just want to make sure that starting new round sets them nil
		pacemaker.lastRoundTC = &abtypes.TimeoutCert{}
		pacemaker.currentQC = &abtypes.QuorumCert{}
		pacemaker.voteSent = &abdrc.VoteMsg{}
		pacemaker.timeoutVote = &abdrc.TimeoutMsg{}

		pacemaker.startNewRound(6)
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
		require.NotNil(t, pacemaker.ticker)
	})

	t.Run("unread event from previous round is cancelled", func(t *testing.T) {
		// when starting new round (unread) event of the previous round must be
		// cancelled (ie disappear from event chan)
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		pacemaker.Reset(4, nil, nil)
		defer pacemaker.Stop()

		// Reset started new round, wait until timeout without consuming events
		time.Sleep(roundTO + minRoundLen/2)

		// starting new round should cancel the event of the previous round so
		// it should take at least minRoundLen before we get pmsRoundMatured
		start := time.Now()
		pacemaker.startNewRound(6)
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
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		pacemaker.Reset(4, nil, nil)
		defer pacemaker.Stop()

		var timeoutCnt, matureCnt, otherCnt int
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
						matureCnt++
					case pmsRoundTimeout:
						timeoutCnt++
					default:
						otherCnt++
					}
				}
			}
		}()

		select {
		case <-time.After((TOcycles + 1) * roundTO):
			t.Error("counting events haven't stopped within timeout")
		case <-done:
		}

		require.Zero(t, otherCnt, `expected the number of "other" events to be zero`)
		require.EqualValues(t, 1, matureCnt, "number of %s events", pmsRoundMatured)
		require.EqualValues(t, TOcycles, timeoutCnt, "number of %s events", pmsRoundTimeout)
	})

	t.Run("timeout vote causes timeout status", func(t *testing.T) {
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		pacemaker.Reset(4, nil, nil)
		defer pacemaker.Stop()

		// register TO vote with quorum condition which allow no faulty nodes - this means
		// that quorum for the round is not possible anymore and PM should go to TO status.
		// timeout event should be in the event channel right away
		qcRound1 := abtypes.NewQuorumCertificate(NewDummyVoteInfo(1, []byte{0, 1, 1}), nil)
		timeoutVoteMsg := NewDummyTimeoutVote(qcRound1, 4, "node1")
		tc, err := pacemaker.RegisterTimeoutVote(timeoutVoteMsg, NewDummyQuorum(3, 0))
		require.NoError(t, err)
		require.Nil(t, tc)

		start := time.Now()
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected to get pmsRoundTimeout event, got %v", e)
			}
		default:
			t.Fatal("expected to get the event immediately")
		}

		// next event must be timeout too
		select {
		case <-time.After(roundTO + roundTO/2):
			t.Fatal("didn't get second event before timeout")
		case e := <-pacemaker.StatusEvents():
			waited := time.Since(start)
			if e != pmsRoundTimeout {
				t.Errorf("expected event %v got %v after %s", pmsRoundTimeout, e, waited)
			}
			if maxDur := roundTO + 50*time.Millisecond; roundTO > waited || waited > maxDur {
				t.Errorf("expected that it will take between %s and %s to receive event, it took %s", roundTO, maxDur, waited)
			}
		}
	})
}

func TestPacemaker_setState(t *testing.T) {
	t.Run("already in pmsRoundTimeout state", func(t *testing.T) {
		roundTO := 500 * time.Millisecond
		pacemaker, err := NewPacemaker(100*time.Millisecond, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		defer pacemaker.Stop()

		// "force" status but do not write into event chan (ie event is already consumed)
		start := time.Now()
		pacemaker.status.Store(uint32(pmsRoundTimeout))
		pacemaker.setState(pmsRoundTimeout, roundTO)
		require.EqualValues(t, pmsRoundTimeout, pacemaker.status.Load())
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected to get pmsRoundTimeout event, got %v", e)
			}
		default:
			t.Error("unexpectedly there is no event in the status event chan")
		}
		// clock should be ticking now with roundTO interval
		select {
		case <-pacemaker.ticker.C:
			if waited := time.Since(start); waited < roundTO {
				t.Errorf("expected to get next tick after %s, got it after %s", roundTO, waited)
			}
		case <-time.After(roundTO + 100*time.Millisecond):
			t.Error("haven't got another tick from clock")
		}
	})

	t.Run("switching to pmsRoundTimeout state", func(t *testing.T) {
		roundTO := 500 * time.Millisecond
		pacemaker, err := NewPacemaker(100*time.Millisecond, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		defer pacemaker.Stop()

		// set current status something other than pmsRoundTimeout
		start := time.Now()
		pacemaker.status.Store(uint32(pmsRoundMatured))
		pacemaker.statusChan <- pmsRoundMatured

		pacemaker.setState(pmsRoundTimeout, roundTO)
		require.EqualValues(t, pmsRoundTimeout, pacemaker.status.Load())
		select {
		case e := <-pacemaker.StatusEvents():
			if e != pmsRoundTimeout {
				t.Errorf("expected to get pmsRoundTimeout event, got %v", e)
			}
		default:
			t.Error("unexpectedly there is no event in the status event chan")
		}
		// seting status must have reset the clock so we should see a tick after roundTO
		select {
		case <-pacemaker.ticker.C:
			if waited := time.Since(start); waited < roundTO {
				t.Errorf("expected to get next tick after %s, got it after %s", roundTO, waited)
			}
		case <-time.After(roundTO + 100*time.Millisecond):
			t.Error("haven't got another tick from clock")
		}
	})
}

func TestPacemaker_Reset(t *testing.T) {
	t.Run("simulate start with last round TC", func(t *testing.T) {
		// test do fields which need to be reset when new round starts do
		// get reset and fields which need to retain their value do
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		tc := &abtypes.TimeoutCert{
			Timeout: &abtypes.Timeout{
				Round: 5,
			},
		}
		pacemaker.Reset(4, tc, nil)
		require.EqualValues(t, 6, pacemaker.GetCurrentRound())
		require.NotNil(t, pacemaker.lastRoundTC)
	})
	t.Run("simulate start with TC from past", func(t *testing.T) {
		// test do fields which need to be reset when new round starts do
		// get reset and fields which need to retain their value do
		minRoundLen := 500 * time.Millisecond
		roundTO := time.Second
		pacemaker, err := NewPacemaker(minRoundLen, roundTO, observability.NOPMetrics())
		require.NoError(t, err)
		tc := &abtypes.TimeoutCert{
			Timeout: &abtypes.Timeout{
				Round: 3,
			},
		}
		pacemaker.Reset(4, tc, nil)
		require.EqualValues(t, 5, pacemaker.GetCurrentRound())
		require.Nil(t, pacemaker.lastRoundTC)
	})
}
