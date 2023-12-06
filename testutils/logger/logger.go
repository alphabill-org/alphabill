package logger

import (
	"io"
	"log/slog"
	"math"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/neilotoole/slogt"
)

/*
NOP returns a logger which doesn't log (ie /dev/null).
Use it for tests where valid logger is needed but it's output is not needed.
*/
func NOP() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo + math.MinInt}))
}

/*
New returns logger for test t on debug level.
*/
func New(t testing.TB) *slog.Logger {
	return NewLvl(t, slog.LevelDebug)
}

/*
NewLvl returns logger for test t on level "level".

First part of the log line is source location which is invalid for messages logged by the
logger (they are correct for the t.Log, t.Error etc calls). Fix needs support from the Go
testing lib (see https://github.com/golang/go/issues/59928).
*/
func NewLvl(t testing.TB, level slog.Level) *slog.Logger {
	cfg := defaultLogCfg()
	cfg.Level = level.String()
	return newLogger(t, cfg)
}

func newLogger(t testing.TB, cfg logger.LogConfiguration) *slog.Logger {
	opt := slogt.Factory(func(w io.Writer) slog.Handler {
		h, err := cfg.Handler(w)
		if err != nil {
			t.Fatalf("creating handler for logger: %v", err)
			return nil
		}
		return h
	})
	return slogt.New(t, opt)
}

func defaultLogCfg() logger.LogConfiguration {
	lvl := os.Getenv("AB_TEST_LOG_LEVEL")
	if lvl == "" {
		lvl = slog.LevelDebug.String()
	}
	return logger.LogConfiguration{
		Level:        lvl,
		Format:       "console",
		TimeFormat:   "15:04:05.0000",
		PeerIDFormat: "short",
		// slogt is logging into bytes.Buffer so can't use w to detect
		// is the destination console or not (ie for color support).
		// So by default use colors unless env var disables it.
		ConsoleSupportsColor: func(w io.Writer) bool { return os.Getenv("AB_TEST_LOG_NO_COLORS") != "true" },
	}
}

/*
LoggerBuilder returns "logger factory" for test t.

Factory function returned by LoggerBuilder returns the same logger for all calls.
*/
func LoggerBuilder(t testing.TB) func(*logger.LogConfiguration) (*slog.Logger, error) {
	logr := newLogger(t, defaultLogCfg())
	return func(*logger.LogConfiguration) (*slog.Logger, error) {
		return logr, nil
	}
}
