package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"go.opentelemetry.io/otel/trace"
)

const (
	LevelTrace slog.Level = slog.LevelDebug - 4
	// levelNone is used internally to disable logging
	levelNone slog.Level = math.MinInt
)

// valid output Format values
const (
	fmtTEXT    = "text"
	fmtJSON    = "json"
	fmtECS     = "ecs" // Elastic Common Schema
	fmtCONSOLE = "console"
	fmtWALLET  = "wallet" // wallet CLI logs to console
)

func New(cfg *LogConfiguration) (*slog.Logger, error) {
	cfg.bridgeOldCfg()

	out, err := filenameToWriter(cfg.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("creating writer for log output: %w", err)
	}

	h, err := cfg.Handler(out)
	if err != nil {
		return nil, fmt.Errorf("creating logger handler: %w", err)
	}
	return slog.New(h), nil
}

type LogConfiguration struct {
	Level           string `yaml:"defaultLevel"`
	Format          string `yaml:"format"`
	OutputPath      string `yaml:"outputPath"`
	TimeFormat      string `yaml:"timeFormat"`
	PeerIDFormat    string `yaml:"peerIdFormat"`
	ShowGoroutineID *bool  `yaml:"showGoroutineID"`

	// when Format==console this func will be used to determine
	// whether to use color codes in log output
	ConsoleSupportsColor func(io.Writer) bool

	// old cfg fields, will be removed eventually

	ConsoleFormat *bool `yaml:"consoleFormat"` // -> Format
	ShowNodeID    *bool `yaml:"showNodeID"`    // -> PeerIDFormat
}

func (cfg *LogConfiguration) Handler(out io.Writer) (slog.Handler, error) {
	// init defaults for everything still unassigned...
	cfg.initDefaults()

	var h slog.Handler
	switch strings.ToLower(cfg.Format) {
	case fmtTEXT:
		h = slog.NewTextHandler(out, cfg.handlerOptions())
	case fmtJSON, fmtECS:
		h = slog.NewJSONHandler(out, cfg.handlerOptions())
	case fmtCONSOLE:
		h = tint.NewHandler(out, &tint.Options{
			Level:      cfg.logLevel(),
			NoColor:    cfg.ConsoleSupportsColor == nil || !cfg.ConsoleSupportsColor(out),
			TimeFormat: cfg.TimeFormat,
			AddSource:  true,
			ReplaceAttr: composeAttrFmt(
				formatPeerIDAttr(cfg.PeerIDFormat),
				formatDataAttrAsJSON,
			),
		})
	case fmtWALLET:
		h = tint.NewHandler(out, &tint.Options{
			Level:       cfg.logLevel(),
			NoColor:     cfg.ConsoleSupportsColor == nil || !cfg.ConsoleSupportsColor(out),
			TimeFormat:  cfg.TimeFormat,
			AddSource:   false,
			ReplaceAttr: formatAttrWallet,
		})
	default:
		return nil, fmt.Errorf("unknown log format %q", cfg.Format)
	}

	h = NewABHandler(h, cfg.ShowGoroutineID == nil || *cfg.ShowGoroutineID)

	return h, nil
}

func (cfg *LogConfiguration) handlerOptions() *slog.HandlerOptions {
	opt := &slog.HandlerOptions{
		AddSource: true,
		Level:     cfg.logLevel(),
	}

	switch cfg.Format {
	case fmtTEXT:
		opt.ReplaceAttr = composeAttrFmt(
			formatTimeAttr(cfg.TimeFormat),
			formatPeerIDAttr(cfg.PeerIDFormat),
		)
	case fmtJSON:
		opt.ReplaceAttr = composeAttrFmt(
			formatTimeAttr(cfg.TimeFormat),
			formatPeerIDAttr(cfg.PeerIDFormat),
		)
	case fmtECS:
		opt.ReplaceAttr = composeAttrFmt(
			formatTimeAttr(cfg.TimeFormat),
			formatAttrECS,
		)
	default:
		return nil
	}

	return opt
}

/*
initDefaults assigns default value to the fields which are unassigned.
*/
func (cfg *LogConfiguration) initDefaults() {
	if cfg.Level == "" {
		cfg.Level = slog.LevelInfo.String()
	}
	if cfg.Format == "" {
		cfg.Format = fmtECS
	}

	if cfg.TimeFormat == "" {
		switch cfg.Format {
		case fmtCONSOLE:
			cfg.TimeFormat = "15:04:05.0000"
		default:
			cfg.TimeFormat = "2006-01-02T15:04:05.0000Z0700"
		}
	}

	if cfg.PeerIDFormat == "" && cfg.Format == fmtCONSOLE {
		cfg.PeerIDFormat = "short"
	}

	if cfg.ConsoleSupportsColor == nil {
		cfg.ConsoleSupportsColor = func(out io.Writer) bool {
			f, ok := out.(interface{ Fd() uintptr })
			return ok && isatty.IsTerminal(f.Fd())
		}
	}

	if cfg.Format == fmtWALLET {
		noGoId := false
		cfg.ShowGoroutineID = &noGoId
	}
}

/*
bridgeOldCfg assigns values to "new style" cfg fields based on "old style" fields.
This is needed only until old logger/config has been refactored out
*/
func (cfg *LogConfiguration) bridgeOldCfg() {
	if cfg.Format == "" {
		if cfg.ConsoleFormat != nil && *cfg.ConsoleFormat {
			cfg.Format = fmtCONSOLE
		} else {
			cfg.Format = fmtECS
		}
	}

	if cfg.PeerIDFormat == "" && cfg.ShowNodeID != nil {
		if !*cfg.ShowNodeID {
			cfg.PeerIDFormat = "none"
		}
	}
}

func (cfg *LogConfiguration) logLevel() slog.Level {
	if cfg.OutputPath == "discard" || cfg.OutputPath == os.DevNull {
		return levelNone
	}

	switch strings.ToLower(cfg.Level) {
	case "warning":
		return slog.LevelWarn
	case "trace":
		return LevelTrace
	case "none":
		return levelNone
	}

	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(cfg.Level))
	return lvl
}

func filenameToWriter(name string) (io.Writer, error) {
	switch strings.ToLower(name) {
	case "stdout":
		return os.Stdout, nil
	case "stderr", "":
		return os.Stderr, nil
	case "discard", os.DevNull:
		return io.Discard, nil
	default:
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return nil, fmt.Errorf("create dir %q for log output: %w", filepath.Dir(name), err)
		}
		file, err := os.OpenFile(filepath.Clean(name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // -rw-------
		if err != nil {
			return nil, fmt.Errorf("open file %q for log output: %w", name, err)
		}
		return file, nil
	}
}

/*
ABHandler is a slog handler which does some AB specific processing to the log:
  - adds goroutine id field to the log record (if flag is set);
  - adds trace/span id attributes (if present in the context);
*/
type ABHandler struct {
	handler     slog.Handler
	goroutineID bool
}

func NewABHandler(h slog.Handler, goroutineID bool) *ABHandler {
	// Optimization: avoid chains of ABHandler
	if lh, ok := h.(*ABHandler); ok {
		h = lh.Handler()
	}
	return &ABHandler{h, goroutineID}
}

func (h *ABHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.goroutineID {
		r.AddAttrs(slog.Uint64(GoIDKey, goroutineID()))
	}

	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.HasTraceID() {
		r.AddAttrs(slog.String(traceID, spanCtx.TraceID().String()))
		if spanCtx.HasSpanID() {
			r.AddAttrs(slog.String(spanID, spanCtx.SpanID().String()))
		}
	}

	return h.handler.Handle(ctx, r)
}

func (h *ABHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *ABHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewABHandler(h.handler.WithAttrs(attrs), h.goroutineID)
}

func (h *ABHandler) WithGroup(name string) slog.Handler {
	return NewABHandler(h.handler.WithGroup(name), h.goroutineID)
}

func (h *ABHandler) Handler() slog.Handler {
	return h.handler
}

/*
NewRoundHandler creates slog handler which adds "round" attribute to each log record
by calling the provided callback.
Meant to be used with components which do have concept of current round, ie shard
validator or RootChain node.
*/
func NewRoundHandler(h slog.Handler, getRound func() uint64) *RoundHandler {
	// Optimization: avoid chains of RoundHandler
	if lh, ok := h.(*RoundHandler); ok {
		h = lh.Handler()
	}
	return &RoundHandler{h, getRound}
}

type RoundHandler struct {
	handler slog.Handler
	round   func() uint64
}

func (h *RoundHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.Uint64(RoundKey, h.round()))

	return h.handler.Handle(ctx, r)
}

func (h *RoundHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *RoundHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewRoundHandler(h.handler.WithAttrs(attrs), h.round)
}

func (h *RoundHandler) WithGroup(name string) slog.Handler {
	return NewRoundHandler(h.handler.WithGroup(name), h.round)
}

func (h *RoundHandler) Handler() slog.Handler {
	return h.handler
}
