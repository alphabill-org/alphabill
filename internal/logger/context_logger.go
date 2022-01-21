package logger

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type (
	ContextLogger struct {
		zeroLogger      *zerolog.Logger
		level           LogLevel
		context         Context
		showGoroutineID bool
	}

	Context map[string]interface{}
)

// newContextLogger creates the logger, but doesn't initialize it yet.
// This is needed, so loggers could be created in var phase. But the global log configuration added later.
func newContextLogger(level LogLevel, context Context, showGoroutineID bool) *ContextLogger {
	return &ContextLogger{
		zeroLogger:      nil,
		level:           level,
		context:         context,
		showGoroutineID: showGoroutineID,
	}
}

// init creates the zerologger instance with attributes set in the constructor.
func (c *ContextLogger) init() {
	c.update(c.level, c.context, c.showGoroutineID)
	InitializeGlobalLogger()
}

func (c *ContextLogger) update(level LogLevel, context Context, showGoroutineID bool) {
	c.level = level
	c.showGoroutineID = showGoroutineID

	zeroLogger := log.Level(toZeroLevel(level))
	for key, value := range context {
		zeroLogger = zeroLogger.With().Interface(key, value).Logger()
	}
	if showGoroutineID {
		zeroLogger = zeroLogger.Hook(goRoutineIDHook{})
	}
	c.zeroLogger = &zeroLogger
}

func (c *ContextLogger) Trace(format string, args ...interface{}) {
	if c.zeroLogger == nil {
		c.init()
	}
	c.logMessage(c.zeroLogger.Trace(), format, args)
}

func (c *ContextLogger) Debug(format string, args ...interface{}) {
	if c.zeroLogger == nil {
		c.init()
	}
	c.logMessage(c.zeroLogger.Debug(), format, args)
}

func (c *ContextLogger) Info(format string, args ...interface{}) {
	if c.zeroLogger == nil {
		c.init()
	}
	c.logMessage(c.zeroLogger.Info(), format, args)
}

func (c *ContextLogger) Warning(format string, args ...interface{}) {
	if c.zeroLogger == nil {
		c.init()
	}
	c.logMessage(c.zeroLogger.Warn(), format, args)
}

func (c *ContextLogger) Error(format string, args ...interface{}) {
	if c.zeroLogger == nil {
		c.init()
	}
	c.logMessage(c.zeroLogger.Error(), format, args)
}

func (c *ContextLogger) logMessage(event *zerolog.Event, format string, args []interface{}) {
	if len(args) == 0 {
		event.Msg(format)
	} else {
		event.Msgf(format, args...)
	}
}

// ChangeLevel changes the level of the context logger.
func (c *ContextLogger) ChangeLevel(newLevel LogLevel) {
	if c.zeroLogger == nil {
		c.init()
	}
	*c.zeroLogger = c.zeroLogger.Level(toZeroLevel(newLevel))
}

// A hook that adds goroutine ID to the log event
type goRoutineIDHook struct{}

func (h goRoutineIDHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	e.Uint64("GoID", goroutineID())
}

func toZeroLevel(lvl LogLevel) zerolog.Level {
	switch lvl {
	case NONE:
		return zerolog.Disabled
	case TRACE:
		return zerolog.TraceLevel
	case DEBUG:
		return zerolog.DebugLevel
	case INFO:
		return zerolog.InfoLevel
	case WARNING:
		return zerolog.WarnLevel
	case ERROR:
		return zerolog.ErrorLevel
	default:
		panic(fmt.Sprintf("unknown level: %d", lvl))
	}
}
