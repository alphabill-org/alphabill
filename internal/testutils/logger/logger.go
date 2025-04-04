package logger

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"testing"

	"github.com/neilotoole/slogt"

	"github.com/alphabill-org/alphabill/logger"
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

/*
HookedLoggerBuilder creates logger which sends every log message also to "hook".
*/
func HookedLoggerBuilder(t testing.TB, hook func(ctx context.Context, r slog.Record)) func(*logger.LogConfiguration) (*slog.Logger, error) {
	return func(lc *logger.LogConfiguration) (*slog.Logger, error) {
		opt := slogt.Factory(func(w io.Writer) slog.Handler {
			cfg := defaultLogCfg()
			h, err := cfg.Handler(w)
			if err != nil {
				t.Fatalf("creating handler for logger: %v", err)
				return nil
			}
			return NewLoggerHook(h, hook)
		})
		return slogt.New(t, opt), nil
	}
}

func NewLoggerHook(h slog.Handler, hook func(ctx context.Context, r slog.Record)) *logHandler {
	if lh, ok := h.(*logHandler); ok {
		h = lh.Handler()
	}
	return &logHandler{h, hook}
}

type logHandler struct {
	handler slog.Handler
	hook    func(ctx context.Context, r slog.Record)
}

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	defer h.hook(ctx, r)
	return h.handler.Handle(ctx, r)
}

func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewLoggerHook(h.handler.WithAttrs(attrs), h.hook)
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	return NewLoggerHook(h.handler.WithGroup(name), h.hook)
}

func (h *logHandler) Handler() slog.Handler {
	return h.handler
}
