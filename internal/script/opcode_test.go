package script

import (
	"testing"

	"github.com/stretchr/testify/require"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
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
	require.ErrorContains(t, err, "cannot peek into empty stack")
	require.True(t, s.isEmpty())

	// test OP_DUP on non-empty stack
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	require.NoError(t, err)
	require.EqualValues(t, s.size(), 2)
	p1, err := s.pop()
	require.NoError(t, err)
	p2, err := s.pop()
	require.NoError(t, err)
	require.EqualValues(t, p1, p2)
	require.True(t, s.isEmpty())
}

func TestOpHash(t *testing.T) {
	tests := []struct {
		name         string
		stackData    []byte
		opData       []byte
		expectedHash []byte
		expectedErr  string
	}{
		{name: "sha256 ok", stackData: hashData, opData: []byte{HashAlgSha256}, expectedHash: expectedSha256Hash},
		{name: "sha512 ok", stackData: hashData, opData: []byte{HashAlgSha512}, expectedHash: expectedSha512Hash},
		{name: "invalid hash algo", stackData: hashData, opData: []byte{0xff}, expectedErr: "OpHash invalid hash algorithm: 0xff"},
		{name: "opcode data empty", stackData: hashData, opData: []byte{}, expectedErr: "OpHash invalid data: 0x"},
		{name: "opcode data too large", stackData: hashData, opData: []byte{0x01, 0x02}, expectedErr: "OpHash invalid data: 0x0102"},
		{name: "empty stack", stackData: nil, opData: []byte{HashAlgSha256}, expectedErr: "OpHash failed to pop stack"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op, c, s := createContext(OpHash)

			// op_hash expects some data on stack
			if tc.stackData != nil {
				s.push(hashData)
			}

			// hash top of the stack
			err := op.exec(c, tc.opData)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
			}

			// verify correct hash
			if tc.expectedHash != nil {
				actualHash, err := s.pop()
				require.NoError(t, err)
				require.True(t, s.isEmpty())
				require.Equal(t, tc.expectedHash, actualHash)
			}
		})
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
	require.ErrorContains(t, err, "OpEqual failed to pop first stack element")
	require.True(t, s.isEmpty())

	// test OP_EQUAL on partial stack
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	require.ErrorContains(t, err, "OpEqual failed to pop second stack element")
	require.True(t, s.isEmpty())

	// test OP_EQUAL TRUE
	s.push([]byte{0x01})
	s.push([]byte{0x01})
	err = op.exec(c, nil)
	require.NoError(t, err)
	require.Equal(t, 1, s.size())
	result, err := s.popBool()
	require.NoError(t, err)
	require.True(t, result)
	require.True(t, s.isEmpty())

	// test OP_EQUAL FALSE
	s.push([]byte{0x01})
	s.push([]byte{0x02})
	err = op.exec(c, nil)
	require.NoError(t, err)
	require.Equal(t, 1, s.size())
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

	// exec OP_PUSH_BOOL <TRUE>
	err := op.exec(c, []byte{BoolTrue})
	require.NoError(t, err)
	val, err := s.popBool()
	require.NoError(t, err)
	require.True(t, val)
	require.True(t, s.isEmpty())

	// exec OP_PUSH_BOOL <FALSE>
	err = op.exec(c, []byte{BoolFalse})
	require.NoError(t, err)
	val, err = s.popBool()
	require.NoError(t, err)
	require.False(t, val)
	require.True(t, s.isEmpty())

	// exec OP_PUSH_BOOL <invalid byte>
	err = op.exec(c, []byte{0x02})
	require.ErrorContains(t, err, "OpPushBool invalid data")

	// exec OP_PUSH_BOOL <TRUE> <FALSE>
	err = op.exec(c, []byte{BoolTrue, BoolFalse})
	require.ErrorContains(t, err, "OpPushBool invalid data")
}

func createContext(opCode byte) (opCode, *scriptContext, *stack) {
	op := opCodes[opCode]
	c := &scriptContext{
		stack:   &stack{},
		sigData: []byte{},
	}
	return op, c, c.stack
}
