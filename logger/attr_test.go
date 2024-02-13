package logger

import (
	"log/slog"
	"testing"
	"time"

	p2ptest "github.com/libp2p/go-libp2p/core/test"

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

func Test_formatPeerIDAttr(t *testing.T) {
	lineKey := "peerId"
	peerId, err := p2ptest.RandPeerID()
	require.NoError(t, err)

	noFmt := formatPeerIDAttr("none")
	lineFmt := noFmt(nil, slog.Any(lineKey, peerId))
	require.Equal(t, slog.Attr{}, lineFmt)

	shortFmt := formatPeerIDAttr("short")
	lineFmt = shortFmt(nil, slog.Any(lineKey, peerId))
	require.Equal(t, lineKey, lineFmt.Key)
	require.NotEmpty(t, lineFmt.Value)

	invalidFmt := formatPeerIDAttr("invalid")
	require.Nil(t, invalidFmt)
}

func Test_formatDataAttrAsJSON(t *testing.T) {
	type SampleData struct {
		Name  string
		Value string
	}

	jsonFmt := formatDataAttrAsJSON(nil, slog.Any(DataKey, &SampleData{Name: "Test", Value: "JSON"}))
	require.Equal(t, DataKey, jsonFmt.Key)
	require.Equal(t, `{"Name":"Test","Value":"JSON"}`, jsonFmt.Value.String())
}

func Test_formatAttrWallet(t *testing.T) {
	sampleData := "sample data"
	walletFmt := formatAttrWallet(nil, slog.Any(slog.LevelKey, sampleData))
	require.Equal(t, slog.LevelKey, walletFmt.Key)
	require.Equal(t, sampleData, walletFmt.Value.String())

	walletFmt = formatAttrWallet(nil, slog.Any(slog.MessageKey, sampleData))
	require.Equal(t, slog.MessageKey, walletFmt.Key)
	require.Equal(t, sampleData, walletFmt.Value.String())

	walletFmt = formatAttrWallet(nil, slog.Any(ErrorKey, sampleData))
	require.Equal(t, ErrorKey, walletFmt.Key)
	require.Equal(t, sampleData, walletFmt.Value.String())

	emptyFmt := formatAttrWallet(nil, slog.Any(slog.TimeKey, sampleData))
	require.Equal(t, slog.Attr{}, emptyFmt)
}

func Test_formatAttrECS(t *testing.T) {
	sampleData := "sample data"
	testFmt := formatAttrECS(nil, slog.Any(slog.MessageKey, sampleData))
	require.Equal(t, "message", testFmt.Key)
	require.Equal(t, sampleData, testFmt.Value.String())

	source := &slog.Source{
		Function: "sample function",
		File:     "sample.spl",
		Line:     10,
	}
	testFmt = formatAttrECS(nil, slog.Any(slog.SourceKey, source))
	require.Equal(t, "log", testFmt.Key)
	require.Equal(t, "origin", testFmt.Value.Group()[0].Key)
	require.Equal(t, "function", testFmt.Value.Group()[0].Value.Group()[0].Key)
	require.Equal(t, source.Function, testFmt.Value.Group()[0].Value.Group()[0].Value.String())
	require.Equal(t, "file", testFmt.Value.Group()[0].Value.Group()[1].Key)
	require.Equal(t, "name", testFmt.Value.Group()[0].Value.Group()[1].Value.Group()[0].Key)
	require.Equal(t, source.File, testFmt.Value.Group()[0].Value.Group()[1].Value.Group()[0].Value.String())
	require.Equal(t, "line", testFmt.Value.Group()[0].Value.Group()[1].Value.Group()[1].Key)
	require.Equal(t, int64(source.Line), testFmt.Value.Group()[0].Value.Group()[1].Value.Group()[1].Value.Int64())

	testFmt = formatAttrECS(nil, slog.Any(NodeIDKey, sampleData))
	require.Equal(t, "service", testFmt.Key)
	require.Equal(t, "node", testFmt.Value.Group()[0].Key)
	require.Equal(t, "name", testFmt.Value.Group()[0].Value.Group()[0].Key)
	require.Equal(t, sampleData, testFmt.Value.Group()[0].Value.Group()[0].Value.String())

	testFmt = formatAttrECS(nil, slog.Any(ErrorKey, sampleData))
	require.Equal(t, "error", testFmt.Key)
	require.Equal(t, "message", testFmt.Value.Group()[0].Key)
	require.Equal(t, sampleData, testFmt.Value.Group()[0].Value.String())

	testFmt = formatAttrECS(nil, slog.Any(DataKey, sampleData))
	require.Equal(t, DataKey, testFmt.Key)
	require.Equal(t, "String", testFmt.Value.Group()[0].Key)
	require.Equal(t, sampleData, testFmt.Value.Group()[0].Value.String())

	testFmt = formatAttrECS(nil, slog.Any(traceID, sampleData))
	require.Equal(t, "trace", testFmt.Key)
	require.Equal(t, "id", testFmt.Value.Group()[0].Key)
	require.Equal(t, sampleData, testFmt.Value.Group()[0].Value.String())

	testFmt = formatAttrECS(nil, slog.Any(spanID, sampleData))
	require.Equal(t, "span", testFmt.Key)
	require.Equal(t, "id", testFmt.Value.Group()[0].Key)
	require.Equal(t, sampleData, testFmt.Value.Group()[0].Value.String())
}

type customLogValuer int

func (clv customLogValuer) LogValue() slog.Value {
	return slog.IntValue(int(clv))
}
