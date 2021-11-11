package logger

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type (
	ContextLogger struct {
		zeroLogger zerolog.Logger
	}

	Context map[string]interface{}
)

func newContextLogger(level LogLevel, context Context, showGoroutineID bool) *ContextLogger {
	cl := &ContextLogger{}
	cl.update(level, context, showGoroutineID)
	return cl
}

func (c *ContextLogger) update(level LogLevel, context Context, showGoroutineID bool) {
	zeroLogger := log.Level(toZeroLevel(level))
	for key, value := range context {
		zeroLogger = zeroLogger.With().Interface(key, value).Logger()
	}
	if showGoroutineID {
		zeroLogger = zeroLogger.Hook(goRoutineIDHook{})
	}
	c.zeroLogger = zeroLogger
}

func (c *ContextLogger) Trace(format string, args ...interface{}) {
	c.logMessage(c.zeroLogger.Trace(), format, args)
}

func (c *ContextLogger) Debug(format string, args ...interface{}) {
	c.logMessage(c.zeroLogger.Debug(), format, args)
}

func (c *ContextLogger) Info(format string, args ...interface{}) {
	c.logMessage(c.zeroLogger.Info(), format, args)
}

func (c *ContextLogger) Warning(format string, args ...interface{}) {
	c.logMessage(c.zeroLogger.Warn(), format, args)
}

func (c *ContextLogger) Error(format string, args ...interface{}) {
	c.logMessage(c.zeroLogger.Error(), format, args)
}

func (c *ContextLogger) logMessage(event *zerolog.Event, format string, args []interface{}) {
	if len(args) == 0 {
		event.Msg(format)
	} else {
		event.Msgf(format, args...)
	}
}

func (c *ContextLogger) ChangeLevel(newLevel LogLevel) {
	c.zeroLogger = c.zeroLogger.Level(toZeroLevel(newLevel))
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
