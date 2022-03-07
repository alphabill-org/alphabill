package errors

import (
	stderr "errors"
)

var (
	// Wrappable error types

	ErrInvalidArgument          = newErrorType("invalid argument")
	ErrInvalidState             = newErrorType("invalid state")
	ErrInvalidHashField         = newErrorType("invalid hash field descriptor")
	ErrSerDesFailed             = newErrorType("(de)serialization failure")
	ErrVerificationFailed       = newErrorType("verification failed")
	ErrVerificationInconsistent = newErrorType("verification inconsistent")
	ErrTimeout                  = newErrorType("timeout")
	ErrPermissionDenied         = newErrorType("permission denied")
	ErrCancelled                = newErrorType("cancelled")
	ErrInconsistentData         = newErrorType("inconsistent data")
	ErrCriticalError            = newErrorType("critical error")
	ErrInsufficientData         = newErrorType("insufficient data")

	// Errors that a returned directly

	ErrOutOfSequence  = newErrorType("out of sequence")
	ErrBufferOverflow = newErrorType("buffer overflow")
	ErrEOS            = newErrorType("end of stream")
	ErrFileNotFound   = newErrorType("file not found")

	ErrNotImplemented = newErrorType("not implemented")
)

func FindErrorType(err error) *AlphabillErrorType {
	var errorType *AlphabillErrorType
	if stderr.As(err, &errorType) {
		return errorType
	}
	return nil
}

// AlphabillErrorType If necessary we can add error codes etc. to this type later on
type AlphabillErrorType struct {
	s string
}

func (e *AlphabillErrorType) Error() string {
	return e.s
}

func newErrorType(s string) *AlphabillErrorType {
	return &AlphabillErrorType{
		s: s,
	}
}
