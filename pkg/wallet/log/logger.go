package log

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"runtime"
	"strconv"
	"sync"
)

// Priority is the log level
type Priority uint

const (
	// NONE Logging is turned off.
	NONE Priority = iota

	// ERROR log level - unrecoverable fatal errors only - gasp of
	// death - code cannot continue and will terminate.
	ERROR

	// WARNING log level - changes in state that affects the service
	// degradation.
	WARNING

	// NOTICE log level - changes in state that do not necessarily
	// cause service degradation.
	NOTICE

	// INFO log level - events that have no effect on service, but
	// can aid in performance, status and statistics monitoring.
	INFO

	// DEBUG log level - events generated to aid in debugging,
	// application flow and detailed service troubleshooting.
	DEBUG
)

var logPrefix = []string{
	NONE:    "[?]",
	ERROR:   "[E]",
	WARNING: "[W]",
	NOTICE:  "[N]",
	INFO:    "[I]",
	DEBUG:   "[D]",
}

var Levels = map[string]Priority{
	"NONE":    NONE,
	"ERROR":   ERROR,
	"WARNING": WARNING,
	"NOTICE":  NOTICE,
	"INFO":    INFO,
	"DEBUG":   DEBUG,
}

// A WriterLogger represents an active logging object that generates lines of
// output to an io.Writer.
// It is a wrapper object for the standard library log.Logger.
type WriterLogger struct {
	log       *stdlog.Logger
	priority  Priority
	calldepth int
	mu        sync.Mutex
}

const (
	logTimeFormat = stdlog.Ldate | stdlog.Ltime | stdlog.Lmicroseconds | stdlog.LUTC
	logFileFormat = stdlog.Lshortfile
)

// New creates a new WriterLogger. Priority set the internal log level. Higher level
// will have greater impact on the performance. The log entries are written
// to the output. In case output is not provided the log is written to stdout.
// Return a new WriterLogger object, or error.
func New(priority Priority, output io.Writer) (*WriterLogger, error) {
	if priority == NONE {
		return nil, errors.New("invalid logging level")
	}

	writer := output
	if output == nil {
		writer = os.Stdout
	}

	return &WriterLogger{
		log:       stdlog.New(writer, "", logTimeFormat|logFileFormat),
		priority:  priority,
		calldepth: 4, // Default calldepth for sdk package level.
	}, nil
}

// SetCalldepth is setter for stack call depth. Used to recover the PC and is
// provided for generality. By default, is set to 4.
func (l *WriterLogger) SetCalldepth(d int) {
	if l == nil {
		return
	}
	l.calldepth = d
}

// Debug for debug level logging. Events generated to aid in debugging,
// application flow and detailed service troubleshooting.
func (l *WriterLogger) Debug(v ...interface{}) {
	if l == nil {
		return
	}
	l.logMessage(DEBUG, v...)
}

// Info for info level logging. Events that have no effect on service,
// but can aid in performance, status and statistics monitoring.
func (l *WriterLogger) Info(v ...interface{}) {
	if l == nil {
		return
	}
	l.logMessage(INFO, v...)
}

// Notice for info level logging. Changes in state that do not necessarily
// cause service degradation.
func (l *WriterLogger) Notice(v ...interface{}) {
	if l == nil {
		return
	}
	l.logMessage(NOTICE, v...)
}

// Warning for warning level logging. Changes in state that affects the
// service degradation.
func (l *WriterLogger) Warning(v ...interface{}) {
	if l == nil {
		return
	}
	l.logMessage(WARNING, v...)
}

// Error for error level logging. Unrecoverable fatal errors only - gasp of
// death - code cannot continue and will terminate.
func (l *WriterLogger) Error(v ...interface{}) {
	if l == nil {
		return
	}
	l.logMessage(ERROR, v...)
}

func (l *WriterLogger) logMessage(p Priority, v ...interface{}) {
	if l == nil || l.priority < p {
		return
	}

	id := goroutineID()
	prefix := logPrefix[p]

	l.mu.Lock()
	defer l.mu.Unlock()

	l.log.SetPrefix(fmt.Sprintf("%s{%04x}", prefix, id))

	/* Logging error is ignored by intention. */
	err := l.log.Output(l.calldepth, fmt.Sprint(v...))
	if err != nil {
		// ignore error
	}
}

// Hackish way to get the goroutine id.
func goroutineID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}
