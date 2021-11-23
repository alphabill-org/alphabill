package script

import (
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/assert"
	"testing"
)

var hashData = []byte{0x01}

// SHA256 hash of hashData
var expectedHash = []byte{
	0x4b, 0xf5, 0x12, 0x2f, 0x34, 0x45, 0x54, 0xc5, 0x3b, 0xde, 0x2e, 0xbb, 0x8c,
	0xd2, 0xb7, 0xe3, 0xd1, 0x60, 0xa, 0xd6, 0x31, 0xc3, 0x85, 0xa5, 0xd7, 0xcc,
	0xe2, 0x3c, 0x77, 0x85, 0x45, 0x9a}

func TestOpDup(t *testing.T) {
	op, c, s := createContext(OpDup)

	// test OP_DUP on empty stack
	err := op.exec(c, nil)
	assert.Equal(t, errPeekEmptyStack, err)
	assert.True(t, s.isEmpty())

	// test OP_DUP on non empty stack
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	assert.Nil(t, err)
	assert.EqualValues(t, s.size(), 2)
	p1, err := s.pop()
	p2, err := s.pop()
	assert.EqualValues(t, p1, p2)
	assert.True(t, s.isEmpty())
}

func TestOpHash(t *testing.T) {
	op, c, s := createContext(OpHash)

	// op_hash expects some data on stack
	s.push(hashData)

	// hash top of stack with SHA256
	err := op.exec(c, []byte{HashAlgSha256})
	assert.Nil(t, err)
	assert.EqualValues(t, 1, s.size())

	// verify correct hash
	hash, err := s.pop()
	assert.Nil(t, err)
	assert.True(t, s.isEmpty())
	assert.Equal(t, expectedHash, hash)
}

func TestOpPushHash(t *testing.T) {
	op, c, s := createContext(OpPushHash)

	data := []byte{HashAlgSha256}
	data = append(data, expectedHash...)

	err := op.exec(c, data)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, s.size())

	hash, err := s.pop()
	assert.Nil(t, err)
	assert.True(t, s.isEmpty())
	assert.Equal(t, expectedHash, hash)
}

func TestOpPushPubKey(t *testing.T) {
	op, c, s := createContext(OpPushPubKey)

	_, pubKey := testsig.SignBytes(t, hashData)

	opData := []byte{SigSchemeSecp256k1}
	opData = append(opData, pubKey...)
	err := op.exec(c, opData)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, s.size())

	actualPubKey, err := s.pop()
	assert.Nil(t, err)
	assert.True(t, s.isEmpty())
	assert.Equal(t, pubKey, actualPubKey)
}

func TestOpPushSig(t *testing.T) {
	op, c, s := createContext(OpPushSig)

	sig, _ := testsig.SignBytes(t, hashData)

	data := []byte{SigSchemeSecp256k1}
	data = append(data, sig...)
	err := op.exec(c, data)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, s.size())

	actualSig, err := s.pop()
	assert.Nil(t, err)
	assert.True(t, s.isEmpty())
	assert.Equal(t, sig, actualSig)
}

func TestOpCheckSig(t *testing.T) {
	op, c, s := createContext(OpCheckSig)

	// create signature
	sig, pubKey := testsig.SignBytes(t, hashData)

	// add sig, pubKey to stack
	c.sigData = hashData
	s.push(sig)
	s.push(pubKey)

	// verify signature on stack
	err := op.exec(c, []byte{SigSchemeSecp256k1})
	assert.Nil(t, err)
	assert.EqualValues(t, 1, s.size())
	sigVerifyResult, err := s.popBool()
	assert.Nil(t, err)
	assert.True(t, s.isEmpty())
	assert.True(t, sigVerifyResult)
}

func TestOpEqual(t *testing.T) {
	op, c, s := createContext(OpEqual)

	// test OP_EQUAL on empty stack
	err := op.exec(c, nil)
	assert.Equal(t, errPopEmptyStack, err)
	assert.True(t, s.isEmpty())

	// test OP_EQUAL on partial stack
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	assert.Equal(t, errPopEmptyStack, err)
	assert.True(t, s.isEmpty())

	// test OP_EQUAL TRUE
	s.push([]byte{0x01})
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, s.size())
	result, err := s.popBool()
	assert.Nil(t, err)
	assert.True(t, result)
	assert.True(t, s.isEmpty())

	// test OP_EQUAL FALSE
	s.push([]byte{0x01})
	s.push([]byte{0x02})
	err = op.exec(c, nil)
	assert.Nil(t, err)
	assert.EqualValues(t, 1, s.size())
	result, err = s.popBool()
	assert.Nil(t, err)
	assert.False(t, result)
	assert.True(t, s.isEmpty())
}

func TestOpVerify(t *testing.T) {
	op, c, s := createContext(OpVerify)

	// test OP_VERIFY removes TRUE value from stack
	s.push([]byte{0x01})
	err := op.exec(c, nil)
	assert.Nil(t, err)
	assert.True(t, s.isEmpty())
}

func TestOpPushBool(t *testing.T) {
	op, c, s := createContext(OpPushBool)
	err := op.exec(c, []byte{BoolTrue})
	assert.Nil(t, err)
	val, err := s.popBool()
	assert.True(t, val)
	assert.Nil(t, err)
	assert.True(t, s.isEmpty())
}

func createContext(opCode byte) (opCode, *scriptContext, *stack) {
	op := opCodes[opCode]
	c := &scriptContext{
		stack:   &stack{},
		sigData: []byte{},
	}
	s := c.stack
	return op, c, s
}
