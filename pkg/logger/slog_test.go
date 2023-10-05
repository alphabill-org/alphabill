package logger

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

func Test_formatTimeAttr(t *testing.T) {
	t.Run("empty format string", func(t *testing.T) {
		f := formatTimeAttr("")
		require.Nil(t, f)
	})

	t.Run("format: none", func(t *testing.T) {
		f := formatTimeAttr("none")
		require.NotNil(t, f)
		now := time.Now()

		a := f(nil, slog.Time(slog.TimeKey, now))
		require.Equal(t, slog.Attr{}, a)

		// when not time key value is preserved
		a = f(nil, slog.Time("foo", now))
		require.True(t, a.Equal(slog.Time("foo", now)))
	})

	t.Run("format: format string", func(t *testing.T) {
		f := formatTimeAttr("15:04:05.0000")
		require.NotNil(t, f)

		// zero time is not changed
		a := f(nil, slog.Time(slog.TimeKey, time.Time{}))
		require.Equal(t, slog.Time(slog.TimeKey, time.Time{}), a)

		// valid time is converted to string representation
		now := time.Now()
		a = f(nil, slog.Time(slog.TimeKey, now))
		require.Equal(t, now.Format("15:04:05.0000"), a.Value.String())

		// value is of wrong type for time key
		require.Panics(t, func() { f(nil, slog.Int(slog.TimeKey, 42)) })

		// when not time key value is not altered
		a = f(nil, slog.Time("foo", now))
		require.True(t, a.Equal(slog.Time("foo", now)))
	})
}

func Test_composeAttrFmt(t *testing.T) {
	b0 := func(groups []string, a slog.Attr) slog.Attr { return slog.Int64(a.Key, a.Value.Int64()+1) }
	b1 := func(groups []string, a slog.Attr) slog.Attr { return slog.Int64(a.Key, a.Value.Int64()+2) }
	b2 := func(groups []string, a slog.Attr) slog.Attr { return slog.Int64(a.Key, a.Value.Int64()+4) }
	b3 := func(groups []string, a slog.Attr) slog.Attr { return slog.Int64(a.Key, a.Value.Int64()+8) }

	require.Nil(t, composeAttrFmt())
	require.Nil(t, composeAttrFmt(nil))
	require.Nil(t, composeAttrFmt(nil, nil))
	require.Nil(t, composeAttrFmt(nil, nil, nil))

	f := composeAttrFmt(b0)
	require.NotNil(t, f)
	a := f(nil, slog.Int64("test", 0))
	require.EqualValues(t, 1, a.Value.Int64())

	f = composeAttrFmt(nil, b1, nil)
	require.NotNil(t, f)
	a = f(nil, slog.Int64("test", 0))
	require.EqualValues(t, 2, a.Value.Int64())

	f = composeAttrFmt(b0, nil, b1)
	require.NotNil(t, f)
	a = f(nil, slog.Int64("test", 0))
	require.EqualValues(t, 3, a.Value.Int64())

	f = composeAttrFmt(b0, b1, b2, nil)
	require.NotNil(t, f)
	a = f(nil, slog.Int64("test", 0))
	require.EqualValues(t, 7, a.Value.Int64())

	f = composeAttrFmt(b0, b1, b2, b3)
	require.NotNil(t, f)
	a = f(nil, slog.Int64("test", 0))
	require.EqualValues(t, 15, a.Value.Int64())

	// funcs are not comparable so when same func is passed multiple times
	// it will be called multiple times...
	f = composeAttrFmt(b3, b3)
	require.NotNil(t, f)
	a = f(nil, slog.Int64("test", 0))
	require.EqualValues(t, 16, a.Value.Int64())
}
