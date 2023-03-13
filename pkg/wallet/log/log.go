// Package log implements a logger interface that is used for logging inside Alphabill Wallet SDK.
//
// In order to enable logging a logger must be registered fist by invoking SetLogger() with an Interface implementation.
// Logging can be disabled by calling SetLogger(nil).
//
// Package provides a basic logging implementation Logger, that generates lines of formatted output to an io.Writer.
package log

import (
	"os"
)

// Deprecated: Use pkg/logger instead.
type Logger interface {
	// Debug for debug priority logging. Events generated to aid in debugging,
	// application flow and detailed service troubleshooting.
	Debug(v ...interface{})

	// Info for info priority logging. Events that have no effect on service,
	// but can aid in performance, status and statistics monitoring.
	Info(v ...interface{})

	// Notice for info priority logging. Changes in state that do not necessarily
	// cause service degradation.
	Notice(v ...interface{})

	// Warning for warning priority logging. Changes in state that affects the service
	// degradation.
	Warning(v ...interface{})

	// Error for error priority logging. Unrecoverable fatal errors only - gasp of
	// death - code cannot continue and will terminate.
	Error(v ...interface{})
}

var logger Logger
var noOp = &noOpLogger{}

// SetLogger initialize a global logger.
// In order to disable logging set the parameter l to nil.
// Deprecated: Use pkg/logger instead.
func SetLogger(l Logger) {
	logger = l
}

// Deprecated: Use pkg/logger instead.
func GetLogger() Logger {
	if logger == nil {
		return noOp
	}
	return logger
}

// Deprecated: Use pkg/logger instead.
func InitStdoutLogger(priority Priority) error {
	if logger == nil {
		l, err := New(priority, os.Stdout)
		if err != nil {
			return err
		}
		SetLogger(l)
	}
	return nil
}

// Debug for debug level logging. Events generated to aid in debugging,
// application flow and detailed service troubleshooting.
// Deprecated: Use pkg/logger instead.
func Debug(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Debug(v...)
}

// Info for info level logging. Events that have no effect on service,
// but can aid in performance, status and statistics monitoring.
// Deprecated: Use pkg/logger instead.
func Info(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Info(v...)
}

// Notice for info level logging. Changes in state that do not necessarily
// cause service degradation.
// Deprecated: Use pkg/logger instead.
func Notice(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Notice(v...)
}

// Warning for warning level logging. Changes in state that affects the
// service degradation.
// Deprecated: Use pkg/logger instead.
func Warning(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Warning(v...)
}

// Error for error level logging. Unrecoverable fatal errors only - gasp of
// death - code cannot continue and will terminate.
// Deprecated: Use pkg/logger instead. instead.
func Error(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Error(v...)
}

type noOpLogger struct{}

func (l *noOpLogger) Debug(...interface{})   {}
func (l *noOpLogger) Info(...interface{})    {}
func (l *noOpLogger) Notice(...interface{})  {}
func (l *noOpLogger) Warning(...interface{}) {}
func (l *noOpLogger) Error(...interface{})   {}
