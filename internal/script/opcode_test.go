package script

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOpEqual(t *testing.T) {
	opEqual := opCodes[OP_EQUAL]
	context := &context{
		stack:   &stack{},
		sigData: []byte{},
	}
	stack := context.stack

	// test OP_EQUAL on empty stack
	err := opEqual.exec(context, nil)
	assert.Equal(t, errPopEmptyStack, err)
	assert.True(t, stack.isEmpty())

	// test OP_EQUAL on partial stack
	stack.push([]byte{0x01})
	err = opEqual.exec(context, nil)
	assert.Equal(t, errPopEmptyStack, err)
	assert.True(t, stack.isEmpty())

	// test OP_EQUAL TRUE
	stack.push([]byte{0x01})
	stack.push([]byte{0x01})
	err = opEqual.exec(context, nil)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, stack.size())
	result, err := stack.popBool()
	assert.Nil(t, err)
	assert.True(t, result)
	assert.True(t, stack.isEmpty())

	// test OP_EQUAL FALSE
	stack.push([]byte{0x01})
	stack.push([]byte{0x02})
	err = opEqual.exec(context, nil)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, stack.size())
	result, err = stack.popBool()
	assert.Nil(t, err)
	assert.False(t, result)
	assert.True(t, stack.isEmpty())
}

func TestOpDup(t *testing.T) {
	opDup := opCodes[OP_DUP]
	context := &context{
		stack:   &stack{},
		sigData: []byte{},
	}
	stack := context.stack

	// test OP_DUP on empty stack
	err := opDup.exec(context, nil)
	assert.Equal(t, errPeekEmptyStack, err)
	assert.True(t, stack.isEmpty())

	// test OP_DUP on non empty stack
	stack.push([]byte{0x01})
	err = opDup.exec(context, nil)
	assert.Nil(t, err)
	assert.EqualValues(t, stack.size(), 2)
	p1, err := stack.pop()
	p2, err := stack.pop()
	assert.EqualValues(t, p1, p2)
	assert.True(t, stack.isEmpty())
}
