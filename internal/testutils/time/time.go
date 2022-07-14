package testtime

import (
	"bytes"
	"fmt"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/errors"

	"github.com/stretchr/testify/require"
)

const (
	timestampFormat = "15:04:05.999"
)

// MustRunInTime executes the function and reports error if it lasts longer than max
func MustRunInTime(t testing.TB, max time.Duration, testFunc func(), message ...string) {
	var (
		timeoutChan = time.After(max)
		doneChan    = make(chan bool)
		panicChan   = make(chan error)
	)

	startTime := time.Now()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicChan <- errors.Errorf("%s", r) // Recover with stacktrace
			} else {
				doneChan <- true
			}
		}()
		startTime = time.Now()
		testFunc()
	}()

	select {
	case <-timeoutChan:
		endTime := time.Now()
		msg := ""
		if len(message) > 0 {
			msg = message[0]
		}
		require.FailNowf(t,
			"test timeout",
			"Test timed out, duration=%fs, start=%s, end=%s, message=%s\n\nGoroutine dump: %s",
			max.Seconds(),
			startTime.Format(timestampFormat),
			endTime.Format(timestampFormat),
			msg,
			dumpGoroutineStackTraces())
	case err := <-panicChan:
		require.FailNowf(t, "test panic", "Test panicked: %s", err)
	case <-doneChan:
		break
	}
}

func dumpGoroutineStackTraces() string {
	buf := bytes.Buffer{}
	profile := pprof.Lookup("goroutine")
	err := profile.WriteTo(&buf, 1)
	if err != nil {
		return fmt.Sprintf("failed to dump stack traces: %s", err)
	}
	return buf.String()
}
