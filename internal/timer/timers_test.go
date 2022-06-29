package timer

import (
	"testing"
	"time"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestRestartEndedTimer(t *testing.T) {
	timers := NewTimers()
	timers.Start("t1", 10*time.Millisecond)
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "t1", nt.Name())
		return true
	}, test.WaitDuration, 1)
	timers.Restart("t1")
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "t1", nt.Name())
		return true
	}, test.WaitDuration, 1)
}

func TestRestartRunningTimer(t *testing.T) {
	timers := NewTimers()
	timers.Start("1", 100*time.Millisecond)
	timers.Restart("1")
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "1", nt.Name())
		return true
	}, test.WaitDuration, 1)
	require.Never(t, func() bool {
		<-timers.C
		return false
	}, 100, 1)
	require.Eventually(t, func() bool {
		timers.WaitClose()
		return true
	}, test.WaitDuration, 1)
}

func TestRestartRunningTimer_MultipleTimes(t *testing.T) {
	timers := NewTimers()
	timers.Start("1", 100*time.Millisecond)
	timers.Restart("1")
	timers.Restart("1")
	timers.Restart("1")
	timers.Restart("1")
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "1", nt.Name())
		return true
	}, test.WaitDuration, 1)
	require.Never(t, func() bool {
		<-timers.C
		return false
	}, 100, 1)
	require.Eventually(t, func() bool {
		timers.WaitClose()
		return true
	}, test.WaitDuration, 1)
}

func TestStartMultipleTimers(t *testing.T) {
	timers := NewTimers()
	timers.Start("1", 500*time.Millisecond)
	timers.Start("2", 200*time.Millisecond)
	timers.Start("3", 1000*time.Millisecond)
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "2", nt.Name())
		return true
	}, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "1", nt.Name())
		return true
	}, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "3", nt.Name())
		return true
	}, test.WaitDuration, test.WaitTick)

	timers.Restart("2")
	require.Eventually(t, func() bool {
		nt := <-timers.C
		require.Equal(t, "2", nt.Name())
		return true
	}, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool {
		timers.WaitClose()
		return true
	}, test.WaitDuration, test.WaitTick)
}
