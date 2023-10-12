package logger

import (
	"log/slog"
	"os"
	"testing"
)

func Test_LogConfiguration_logLevel(t *testing.T) {
	var cases = []struct {
		name  string
		level slog.Level
	}{
		{"", slog.LevelInfo},
		{"error", slog.LevelError},
		{"InfO", slog.LevelInfo},
		{"ERROR", slog.LevelError},
		{"WARNING", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"INFO", slog.LevelInfo},
		{"DEBUG", slog.LevelDebug},
		{"TRACE", LevelTrace},
		{"NONE", levelNone},
		{"info-1", slog.LevelInfo - 1},
		{"info+1", slog.LevelInfo + 1},
	}

	for _, tc := range cases {
		cfg := LogConfiguration{Level: tc.name}
		if lvl := cfg.logLevel(); lvl != tc.level {
			t.Errorf("expected %q to return %d (%s) but got %d (%s)", tc.name, tc.level, tc.level, lvl, lvl)
		}
	}

	// special case - when OutputPath is "discard" return levelNone
	cfg := LogConfiguration{Level: "info", OutputPath: "discard"}
	if lvl := cfg.logLevel(); lvl != levelNone {
		t.Errorf("expected %d but got %d for level", levelNone, lvl)
	}

	cfg = LogConfiguration{Level: "info", OutputPath: os.DevNull}
	if lvl := cfg.logLevel(); lvl != levelNone {
		t.Errorf("expected %d but got %d for level", levelNone, lvl)
	}
}
