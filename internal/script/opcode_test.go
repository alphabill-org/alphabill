package script

import (
	"testing"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

var hashData = []byte{0x01}

// SHA256 hash of hashData
var expectedSha256Hash = []byte{
	0x4b, 0xf5, 0x12, 0x2f, 0x34, 0x45, 0x54, 0xc5, 0x3b, 0xde, 0x2e, 0xbb, 0x8c,
	0xd2, 0xb7, 0xe3, 0xd1, 0x60, 0xa, 0xd6, 0x31, 0xc3, 0x85, 0xa5, 0xd7, 0xcc,
	0xe2, 0x3c, 0x77, 0x85, 0x45, 0x9a}

// SHA512 hash of hashData
var expectedSha512Hash = []byte{
	0x7b, 0x54, 0xb6, 0x68, 0x36, 0xc1, 0xfb, 0xdd, 0x13, 0xd2, 0x44, 0x1d, 0x9e,
	0x14, 0x34, 0xdc, 0x62, 0xca, 0x67, 0x7f, 0xb6, 0x8f, 0x5f, 0xe6, 0x6a, 0x46,
	0x4b, 0xaa, 0xde, 0xcd, 0xbd, 0x0, 0x57, 0x6f, 0x8d, 0x6b, 0x5a, 0xc3, 0xbc,
	0xc8, 0x8, 0x44, 0xb7, 0xd5, 0xb, 0x1c, 0xc6, 0x60, 0x34, 0x44, 0xbb, 0xe7,
	0xcf, 0xcf, 0x8f, 0xc0, 0xaa, 0x1e, 0xe3, 0xc6, 0x36, 0xd9, 0xe3, 0x39}

func TestOpDup(t *testing.T) {
	op, c, s := createContext(OpDup)

	// test OP_DUP on empty stack
	err := op.exec(c, nil)
	require.Equal(t, errPeekEmptyStack, err)
	require.True(t, s.isEmpty())

	// test OP_DUP on non empty stack
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	require.NoError(t, err)
	require.EqualValues(t, s.size(), 2)
	p1, err := s.pop()
	p2, err := s.pop()
	require.EqualValues(t, p1, p2)
	require.True(t, s.isEmpty())
}

func TestOpHash(t *testing.T) {
	op, c, s := createContext(OpHash)

	tests := []struct {
		stackData    []byte
		opData       []byte
		expectedHash []byte
		expectedErr  error
	}{
		{stackData: hashData, opData: []byte{HashAlgSha256}, expectedHash: expectedSha256Hash},
		{stackData: hashData, opData: []byte{HashAlgSha512}, expectedHash: expectedSha512Hash},
		{stackData: hashData, opData: []byte{0xff}, expectedErr: errInvalidHashAlgo},
		{stackData: hashData, opData: []byte{}, expectedErr: errInvalidOpcodeData},
		{stackData: hashData, opData: []byte{0x01, 0x02}, expectedErr: errInvalidOpcodeData},
		{stackData: nil, opData: []byte{HashAlgSha256}, expectedErr: errPopEmptyStack},
	}

	for _, tc := range tests {
		// op_hash expects some data on stack
		if tc.stackData != nil {
			s.push(hashData)
		}

		// hash top of the stack
		err := op.exec(c, tc.opData)
		require.Equal(t, tc.expectedErr, err)
		if tc.expectedErr != nil {
			require.True(t, s.isEmpty())
		}

		// verify correct hash
		if tc.expectedHash != nil {
			actualHash, err := s.pop()
			require.NoError(t, err)
			require.True(t, s.isEmpty())
			require.Equal(t, tc.expectedHash, actualHash)
		}
	}
}

func TestOpPushHash(t *testing.T) {
	op, c, s := createContext(OpPushHash)

	data := []byte{HashAlgSha256}
	data = append(data, expectedSha256Hash...)

	err := op.exec(c, data)
	require.NoError(t, err)
	require.EqualValues(t, 1, s.size())

	hash, err := s.pop()
	require.NoError(t, err)
	require.True(t, s.isEmpty())
	require.Equal(t, expectedSha256Hash, hash)
}

func TestOpPushPubKey(t *testing.T) {
	op, c, s := createContext(OpPushPubKey)

	_, pubKey := testsig.SignBytes(t, hashData)

	opData := []byte{SigSchemeSecp256k1}
	opData = append(opData, pubKey...)
	err := op.exec(c, opData)
	require.NoError(t, err)
	require.EqualValues(t, 1, s.size())

	actualPubKey, err := s.pop()
	require.NoError(t, err)
	require.True(t, s.isEmpty())
	require.Equal(t, pubKey, actualPubKey)
}

func TestOpPushSig(t *testing.T) {
	op, c, s := createContext(OpPushSig)

	sig, _ := testsig.SignBytes(t, hashData)

	data := []byte{SigSchemeSecp256k1}
	data = append(data, sig...)
	err := op.exec(c, data)
	require.NoError(t, err)
	require.EqualValues(t, 1, s.size())

	actualSig, err := s.pop()
	require.NoError(t, err)
	require.True(t, s.isEmpty())
	require.Equal(t, sig, actualSig)
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
	require.NoError(t, err)
	require.EqualValues(t, 1, s.size())
	sigVerifyResult, err := s.popBool()
	require.NoError(t, err)
	require.True(t, s.isEmpty())
	require.True(t, sigVerifyResult)
}

func TestOpEqual(t *testing.T) {
	op, c, s := createContext(OpEqual)

	// test OP_EQUAL on empty stack
	err := op.exec(c, nil)
	require.Equal(t, errPopEmptyStack, err)
	require.True(t, s.isEmpty())

	// test OP_EQUAL on partial stack
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	require.Equal(t, errPopEmptyStack, err)
	require.True(t, s.isEmpty())

	// test OP_EQUAL TRUE
	s.push([]byte{0x01})
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, s.size())
	result, err := s.popBool()
	require.NoError(t, err)
	require.True(t, result)
	require.True(t, s.isEmpty())

	// test OP_EQUAL FALSE
	s.push([]byte{0x01})
	s.push([]byte{0x02})
	err = op.exec(c, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, s.size())
	result, err = s.popBool()
	require.NoError(t, err)
	require.False(t, result)
	require.True(t, s.isEmpty())
}

func TestOpVerify(t *testing.T) {
	op, c, s := createContext(OpVerify)

	// test OP_VERIFY removes TRUE value from stack
	s.push([]byte{0x01})
	err := op.exec(c, nil)
	require.NoError(t, err)
	require.True(t, s.isEmpty())
}

func TestOpPushBool(t *testing.T) {
	op, c, s := createContext(OpPushBool)
	err := op.exec(c, []byte{BoolTrue})
	require.NoError(t, err)
	val, err := s.popBool()
	require.True(t, val)
	require.NoError(t, err)
	require.True(t, s.isEmpty())
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
