package logger

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

func Test_dataName(t *testing.T) {
	type myData struct {
		v int
	}
	var clv customLogValuer = 4

	var testCases = []struct {
		value slog.Value
		name  string
	}{
		{value: slog.BoolValue(true), name: "Bool"},
		{value: slog.IntValue(32), name: "Int64"},
		{value: slog.Int64Value(64), name: "Int64"},
		{value: slog.Uint64Value(90), name: "Uint64"},
		{value: slog.Float64Value(1.5), name: "Float64"},
		{value: slog.StringValue("foobar"), name: "String"},
		{value: slog.TimeValue(time.Now()), name: "Time"},
		{value: slog.DurationValue(time.Second), name: "Duration"},
		// slog correctly detects the type even when Any is used
		{value: slog.AnyValue(555), name: "Int64"},
		{value: slog.AnyValue("hi"), name: "String"},
		// struct value and pointer should result in the same name
		{value: slog.AnyValue(myData{42}), name: "logger_myData"},
		{value: slog.AnyValue(&myData{42}), name: "logger_myData"},
		// anonymous struct type will return its def as name
		{value: slog.AnyValue(struct{ n int }{666}), name: "struct { n int }"},
		// type which implements LogValuer
		{value: slog.AnyValue(customLogValuer(2)), name: "logger_customLogValuer"},
		{value: slog.AnyValue(&clv), name: "logger_customLogValuer"},
		// GroupValue will probably cause problems in Elastic :)
		{value: slog.GroupValue(slog.Any("key", "value")), name: "Group"},
	}

	for n, tc := range testCases {
		if name := dataName(tc.value); tc.name != name {
			t.Errorf("[%d] expected %q got %q for %#v", n, tc.name, name, tc.value.Any())
		}
	}
}

type customLogValuer int

func (clv customLogValuer) LogValue() slog.Value {
	return slog.IntValue(int(clv))
}
