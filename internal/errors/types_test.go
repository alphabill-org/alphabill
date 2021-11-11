package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAlphabillErrorType_Error(t *testing.T) {
	expectedMsg := "invalid something"
	errorType := newErrorType(expectedMsg)

	assert.Equal(t, expectedMsg, errorType.Error())
}

func TestAlphabillErrorType_FindErrorType(t *testing.T) {
	wrappedErr := Wrap(Wrap(ErrInvalidHashField, "first"), "second)")
	assert.Equal(t, ErrInvalidHashField, FindErrorType(wrappedErr))

	emptyErr := Wrap(New("non-typed cause error"), "real error")
	assert.Nil(t, FindErrorType(emptyErr))
}
