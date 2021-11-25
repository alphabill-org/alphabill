package bsn

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_Ok(t *testing.T) {
	_, err := New(100)
	require.Nil(t, err)

	t.Fail() // TODO check that one bill was made actually
}
