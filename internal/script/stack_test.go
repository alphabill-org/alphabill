package script

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateEmptyStack_Ok(t *testing.T) {
	s := stack{}
	assert.EqualValues(t, 0, s.size())
}

func TestPushToEmptyStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x53})
	assert.EqualValues(t, 1, s.size())
}

func TestPopFromStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x53})
	top, err := s.pop()
	assert.Equal(t, []byte{0x53}, top)
	assert.Equal(t, nil, err)
}

func TestPopFromEmptyStack_Nok(t *testing.T) {
	s := stack{}
	top, err := s.pop()
	assert.Nil(t, top)
	assert.Equal(t, errPopEmptyStack, err)
}

func TestPeekFromEmptyStack_Nok(t *testing.T) {
	s := stack{}
	peek, err := s.peek()
	assert.NotNil(t, err)
	assert.Nil(t, peek)
}

func TestPeekFromStack_Ok(t *testing.T) {
	s := stack{}
	s.push([]byte{0x01})
	peek, err := s.peek()
	assert.Nil(t, err)
	assert.Equal(t, []byte{0x01}, peek)
	assert.EqualValues(t, 1, s.size())
}

func TestStackIsEmpty_Ok(t *testing.T) {
	s := stack{}
	assert.Equal(t, true, s.isEmpty())
	s.push([]byte{0x01})
	assert.Equal(t, false, s.isEmpty())
}
