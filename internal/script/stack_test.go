package script

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateEmptyStack_Ok(t *testing.T) {
	s := stack{}
	require.EqualValues(t, 0, s.size())
}

func TestPushToEmptyStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x53})
	require.EqualValues(t, 1, s.size())
}

func TestPopFromStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x53})
	top, err := s.pop()
	require.Equal(t, []byte{0x53}, top)
	require.Equal(t, nil, err)
}

func TestPopFromEmptyStack_Nok(t *testing.T) {
	s := stack{}
	top, err := s.pop()
	require.Nil(t, top)
	require.Equal(t, errPopEmptyStack, err)
}

func TestPeekFromEmptyStack_Nok(t *testing.T) {
	s := stack{}
	peek, err := s.peek()
	require.Error(t, err)
	require.Nil(t, peek)
}

func TestPeekFromStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x01})
	peek, err := s.peek()
	require.NoError(t, err)
	require.Equal(t, []byte{0x01}, peek)
	require.EqualValues(t, 1, s.size())
}

func TestStackIsEmpty_Ok(t *testing.T) {
	s := stack{}
	require.Equal(t, true, s.isEmpty())
	s.push([]byte{0x01})
	require.Equal(t, false, s.isEmpty())
}
