package errors

import (
	stderr "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createCauseError() error {
	return New("cause")
}

func TestErrors_WithStacks(t *testing.T) {
	SetGenerateStackTraces(true)
	causedError := createCauseError()

	tests := []struct {
		name                 string
		err                  error
		expectedLinePatterns []string
	}{
		{
			name: "errors.New",
			err:  New("test error"),
			expectedLinePatterns: []string{
				`test error`,
				`at github\.com\/alphabill-org\/alphabill\/internal\/errors\.TestErrors_WithStacks\(.*internal.errors.errors_test\.go:\d+\)`,
			},
		},
		{
			name: "errors.Errorf",
			err:  Errorf("test error with code %d", 5),
			expectedLinePatterns: []string{
				`test error with code 5`,
				`at github\.com\/alphabill-org\/alphabill\/internal\/errors\.TestErrors_WithStacks\(.*internal.errors.errors_test\.go:\d+\)`,
			},
		},
		{
			name: "errors.Wrap",
			err:  Wrap(nil, "parent error"),
			expectedLinePatterns: []string{
				`parent error`,
				`at github\.com\/alphabill-org\/alphabill\/internal\/errors\.TestErrors_WithStacks\(.*internal.errors.errors_test\.go:\d+\)`,
			},
		},
		{
			name: "errors.Wrapf",
			err:  Wrapf(causedError, "parent error with code %d", 10),
			expectedLinePatterns: []string{
				`parent error with code 10`,
				`at github\.com\/alphabill-org\/alphabill\/internal\/errors\.TestErrors_WithStacks\(.*internal.errors.errors_test\.go:\d+\)`,
				`Cause: cause`,
				`at github\.com\/alphabill-org\/alphabill\/internal\/errors\.createCauseError\(.*internal.errors.errors_test\.go:\d+\)`,
				`at github\.com\/alphabill-org\/alphabill\/internal\/errors\.TestErrors_WithStacks\(.*internal.errors.errors_test\.go:\d+\)`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalErrorLines := strings.Split(tt.err.Error(), "\n")
			filteredErrorLines := make([]string, 0)

			// Exclude testing library entries and runtime from stacktrace
			// Might make the tests flaky otherwise (when golang or test harness versions change)
			for _, line := range originalErrorLines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "at testing.") || strings.HasPrefix(line, "at runtime.") {
					continue
				}
				filteredErrorLines = append(filteredErrorLines, line)
			}

			for i, pattern := range tt.expectedLinePatterns {
				if i >= len(filteredErrorLines) {
					assert.Fail(t, fmt.Sprintf(`Line missing in error message: "%s"`, pattern))
					break
				}

				assert.Regexp(t, pattern, filteredErrorLines[i])
			}

			if len(filteredErrorLines) > len(tt.expectedLinePatterns) {
				assert.Fail(t, fmt.Sprintf("Unexpected extra lines in error message: %s", filteredErrorLines[len(tt.expectedLinePatterns):]))
			}
		})
	}
}

func TestErrors_NoStacks(t *testing.T) {
	SetGenerateStackTraces(false)
	causedError := createCauseError()

	tests := []struct {
		name          string
		err           error
		expectedLines []string
	}{
		{
			name: "errors.New",
			err:  New("test error"),
			expectedLines: []string{
				`test error`,
			},
		},
		{
			name: "errors.Errorf",
			err:  Errorf("test error with code %d", 5),
			expectedLines: []string{
				`test error with code 5`,
			},
		},
		{
			name: "errors.Wrap",
			err:  Wrap(nil, "parent error"),
			expectedLines: []string{
				`parent error`,
			},
		},
		{
			name: "errors.Wrapf",
			err:  Wrapf(causedError, "parent error with code %d", 10),
			expectedLines: []string{
				`parent error with code 10`,
				`Cause: cause`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorLines := strings.Split(tt.err.Error(), "\n")

			for i, exLine := range tt.expectedLines {
				if i >= len(errorLines) {
					assert.Fail(t, fmt.Sprintf(`Line missing in error message: "%s"`, exLine))
					break
				}

				assert.Regexp(t, exLine, errorLines[i])
			}

			if len(errorLines) > len(tt.expectedLines) {
				assert.Fail(t, fmt.Sprintf("Unexpected extra lines in error message: %s", errorLines[len(tt.expectedLines):]))
			}
		})
	}
}

func TestErrors_UnWrap(t *testing.T) {
	SetGenerateStackTraces(true)
	causedError := createCauseError()

	tests := []struct {
		name          string
		err           error
		expectedCause error
	}{
		{
			name:          "errors.New",
			err:           New("test error"),
			expectedCause: nil,
		},
		{
			name:          "errors.Errorf",
			err:           Errorf("test error with code %d", 5),
			expectedCause: nil,
		},
		{
			name:          "errors.Wrap",
			err:           Wrap(nil, "parent error"),
			expectedCause: nil,
		},
		{
			name:          "errors.Wrapf",
			err:           Wrapf(causedError, "parent error with code %d", 10),
			expectedCause: causedError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unwrapped := stderr.Unwrap(tt.err)
			assert.Equal(t, tt.expectedCause, unwrapped)
			assert.Equal(t, tt.expectedCause != nil, stderr.Is(tt.err, tt.expectedCause))
		})
	}
}

func TestErrors_AsIs(t *testing.T) {
	err := Wrap(ErrInvalidArgument, "must be postive integer")

	assert.True(t, stderr.Is(err, ErrInvalidArgument))

	var errorType *AlphabillErrorType
	assert.True(t, stderr.As(err, &errorType))
	assert.Same(t, ErrInvalidArgument, errorType)
}

func TestErrors_NilSafety(t *testing.T) {
	nilErr := (*AlphabillError)(nil)
	nilWrappedErr := (*AlphabillError)(nil)

	assert.Equal(t, "", nilErr.Error())
	assert.Equal(t, "", nilWrappedErr.Error())
	assert.Nil(t, nilWrappedErr.Unwrap())
}

type (
	CustomError struct {
		s string
	}
)

func (e *CustomError) Error() string { return e.s }

func TestErrorCausedBy(t *testing.T) {
	var ErrorA = &CustomError{"This is error A"}

	tests := []struct {
		name             string
		err              error
		causedBy         error
		expectedCausedBy bool
	}{
		{
			name:             "no wrap means not caused by",
			err:              New("test error"),
			causedBy:         ErrorA,
			expectedCausedBy: false,
		},
		{
			name:             "nil err means not caused by",
			err:              nil,
			causedBy:         ErrorA,
			expectedCausedBy: false,
		},
		{
			name:             "nil cause means not caused by",
			err:              Wrap(ErrorA, "wrapped error"),
			causedBy:         nil,
			expectedCausedBy: false,
		},
		{
			name:             "wrapping an error will be found",
			err:              Wrap(ErrorA, "wrapped error"),
			causedBy:         ErrorA,
			expectedCausedBy: true,
		},
		{
			name:             "wrapping twice is still found",
			err:              Wrap(Wrap(ErrorA, "wrapped error"), "another step"),
			causedBy:         ErrorA,
			expectedCausedBy: true,
		},
		{
			name:             "error message is important",
			err:              Wrap(New("This is error A"), "bla bla"),
			causedBy:         ErrorA,
			expectedCausedBy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualCausedBy := ErrorCausedBy(tt.err, tt.causedBy)
			assert.Equal(t, tt.expectedCausedBy, actualCausedBy)
		})
	}
}
