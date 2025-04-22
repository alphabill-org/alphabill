package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TryTilCountIs - checks condition after each tick until condition returns true or count is equal to cnt in which case
// the test fails.
// Prefer this helper to require.Evetually when test timeout is small or close to tick timeout.
func TryTilCountIs(t *testing.T, condition func() bool, cnt uint64, tick time.Duration, msgAndArgs ...interface{}) {
	ch := make(chan bool, 1)

	count := uint64(0)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for tick := ticker.C; ; {
		select {
		case <-tick:
			tick = nil
			go func() { ch <- condition() }()
		case v := <-ch:
			if v {
				return
			}
			if count++; count >= cnt {
				assert.Fail(t, "Condition never satisfied", msgAndArgs...)
				t.FailNow()
			}
			tick = ticker.C
		}
	}
}
