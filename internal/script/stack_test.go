package script

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateEmptyStack_Ok(t *testing.T) {
	s := stack{}
	require.Equal(t, 0, s.size())
}

func TestPushToEmptyStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x53})
	require.Equal(t, 1, s.size())
}

func TestPopFromStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x53})
	top, err := s.pop()
	require.NoError(t, err)
	require.Equal(t, []byte{0x53}, top)
}

func TestPopFromEmptyStack_Nok(t *testing.T) {
	s := stack{}
	top, err := s.pop()
	require.ErrorContains(t, err, "cannot pop from empty stack")
	require.Nil(t, top)
}

func TestPeekFromEmptyStack_Nok(t *testing.T) {
	s := stack{}
	peek, err := s.peek()
	require.ErrorContains(t, err, "cannot peek into empty stack")
	require.Nil(t, peek)
}

func TestPeekFromStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x01})
	peek, err := s.peek()
	require.NoError(t, err)
	require.Equal(t, []byte{0x01}, peek)
	require.Equal(t, 1, s.size())
}

func TestStackIsEmpty_Ok(t *testing.T) {
	s := stack{}
	require.True(t, s.isEmpty())
	s.push([]byte{0x01})
	require.False(t, s.isEmpty())
}
