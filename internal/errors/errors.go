package errors

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	// In the future we can add some ENV_VAR check here and disable it by default
	createStackTraces = true
)

// SetGenerateStackTraces Set whether to generate stack traces for errors or not
func SetGenerateStackTraces(generate bool) {
	createStackTraces = generate
}

// AlphabillError is errors specific to Alphabill logic.
type AlphabillError struct {
	message string
	cause   error
	stack   string
}

func (e *AlphabillError) Error() string {
	if e == nil {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(e.message)
	if len(e.stack) != 0 {
		builder.WriteString(fmt.Sprintf("\n%s", padStacktrace(e.stack)))
	}

	if e.cause != nil {
		builder.WriteString(fmt.Sprintf("\nCause: %s", e.cause.Error()))
	}
	return builder.String()
}

func (e *AlphabillError) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *AlphabillError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// New Creates a simple error with message
// The error will also have a stack trace if enabled using errors.SetGenerateStackTraces(generate bool)
// Deprecated: use native go error
func New(message string) error {
	return &AlphabillError{
		message: message,
		cause:   nil,
		stack:   createStack(),
	}
}

// Errorf Creates a simple error from a format and arguments
// The error will also have a stack trace if enabled using errors.SetGenerateStackTraces(generate bool)
// Deprecated: use fmt.Errorf instead
func Errorf(format string, a ...interface{}) error {
	return &AlphabillError{
		message: fmt.Sprintf(format, a...),
		cause:   nil,
		stack:   createStack(),
	}
}

// Wrap Creates an error with message and another error as its cause
// The error will also have a stack trace if enabled using errors.SetGenerateStackTraces(generate bool)
// Deprecated: use native go errors and fmt.Errorf with %w or errors.Join instead
func Wrap(cause error, message string) error {
	return &AlphabillError{
		message: message,
		cause:   cause,
		stack:   createStack(),
	}

}

// Wrapf Creates an error from a format and arguments, along with another error as its cause
// The error will also have a stack trace if enabled using errors.SetGenerateStackTraces(generate bool)
// Deprecated: use native go errors and fmt.Errorf with %w instead
func Wrapf(cause error, format string, a ...interface{}) error {
	msg := fmt.Sprintf(format, a...)
	return &AlphabillError{
		message: msg,
		cause:   cause,
		stack:   createStack(),
	}
}

func createStack() string {
	if !createStackTraces {
		return ""
	}

	const maxStackDepth = 16
	programCounters := make([]uintptr, maxStackDepth)
	n := runtime.Callers(3, programCounters)
	programCounters = programCounters[:n]
	frames := runtime.CallersFrames(programCounters)

	var builder strings.Builder
	for {
		frame, more := frames.Next()
		builder.WriteString(fmt.Sprintf("at %s(%s:%d)\n", frame.Function, frame.File, frame.Line))
		if !more {
			break
		}
	}

	return builder.String()
}

func padStacktrace(stacktrace string) string {
	padChar := "\t"
	return padChar + strings.Join(strings.Split(strings.TrimSpace(stacktrace), "\n"), "\n"+padChar)
}

// ErrorCausedBy checks if any error in error queue matches the given error
// Deprecated: use native error and errors.Is method
func ErrorCausedBy(err error, cause error) bool {
	if err == nil || cause == nil {
		return false
	}
	var expectedMessage, message string
	expectedABErr, ok := cause.(*AlphabillError)
	if ok {
		expectedMessage = expectedABErr.Message()
	} else {
		expectedMessage = cause.Error()
	}

	for {
		abErr, ok := err.(*AlphabillError)
		if ok {
			message = abErr.Message()
		} else {
			message = err.Error()
		}

		if message == expectedMessage {
			return true
		}

		if abErr != nil {
			err = abErr.Unwrap()
			if err == nil {
				return false
			}
		} else {
			return false
		}
	}
}
