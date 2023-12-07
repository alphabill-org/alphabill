package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStack(t *testing.T) {
	var items Stack[*int]
	require.True(t, items.IsEmpty())
	require.Panics(t, func() { items.Pop() })

	items.Push(nil)
	require.False(t, items.IsEmpty())

	var myInt int = 123
	items.Push(&myInt)
	require.Equal(t, len(items), 2)
	require.Equal(t, *items.Pop(), 123)
	require.Equal(t, items.Pop(), (*int)(nil))
}
