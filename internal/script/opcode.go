package script

import (
	"bytes"
	"errors"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
)

type opCode struct {
	value         byte                               // identifier of the op code
	exec          func(*scriptContext, []byte) error // executes opCode logic, modifies the *scriptContext
	getDataLength func(script []byte) (int, error)   // parses the length of data following the given opcode
}

const (
	OpDup        = 0x76
	OpHash       = 0xa8
	OpPushHash   = 0x4f
	OpPushPubKey = 0x55
	OpPushSig    = 0x54
	OpCheckSig   = 0xac
	OpEqual      = 0x87
	OpVerify     = 0x69
	OpPushBool   = 0x51

	// TODO implement below opcodes
	//OP_PUSH_INT64   = 0x01
	//OP_IF           = 0x63
	//OP_ELSE         = 0x67
	//OP_END_IF       = 0x68
	//OP_SWAP         = 0x7c
	//OP_DROP         = 0x75
	//OP_NOT          = 0x91
)

const (
	HashAlgSha256      = 0x01
	HashAlgSha512      = 0x02
	BoolFalse          = 0x00
	BoolTrue           = 0x01
	SigSchemeSecp256k1 = 0x01
)

var opCodes = map[byte]opCode{
	OpPushBool:   {OpPushBool, opPushBool, fixedDataLength(1)},
	OpPushSig:    {OpPushSig, opPushSig, fixedDataLength(66)},
	OpPushPubKey: {OpPushPubKey, opPushPubKey, fixedDataLength(34)},
	OpPushHash:   {OpPushHash, opPushHash, opPushHashDataLength},
	OpHash:       {OpHash, opHash, fixedDataLength(1)},
	OpDup:        {OpDup, opDup, fixedDataLength(0)},
	OpEqual:      {OpEqual, opEqual, fixedDataLength(0)},
	OpVerify:     {OpVerify, opVerify, fixedDataLength(0)},
	OpCheckSig:   {OpCheckSig, opCheckSig, fixedDataLength(1)},
}

var (
	errInvalidOpcodeData = errors.New("invalid opcode data")
	errInvalidHashAlgo   = errors.New("invalid hash algorithm")
	errInvalidSigScheme  = errors.New("invalid sig scheme")
	errInvalidBool       = errors.New("invalid bool")
)

// fixedDataLength returns fixed opCode dataLength or error if data is out of bounds from script
func fixedDataLength(length int) func(script []byte) (int, error) {
	return func(script []byte) (int, error) {
		if len(script) < length {
			return 0, errInvalidOpcodeData
		}
		return length, nil
	}
}

// opPushHashDataLength returns parsed data length of OpPushHash
func opPushHashDataLength(script []byte) (int, error) {
	if len(script) == 0 {
		return 0, errInvalidOpcodeData
	}
	hashAlg := script[0]
	if hashAlg == HashAlgSha256 && len(script) >= 33 {
		return 33, nil
	}
	if hashAlg == HashAlgSha512 && len(script) >= 65 {
		return 65, nil
	}
	return 0, errInvalidOpcodeData
}

// opPushBool pushes bool to stack, returns error if data is not a valid bool
func opPushBool(c *scriptContext, data []byte) error {
	if len(data) != 1 {
		return errInvalidOpcodeData
	}
	if data[0] == BoolFalse || data[0] == BoolTrue {
		c.stack.push(data)
		return nil
	}
	return errInvalidBool
}

// opPushSig pushes sig to the stack. The number of bytes of data to read is determined by first byte <SigScheme> label:
// 0x01 – <secp256k1>, 65 bytes
func opPushSig(c *scriptContext, data []byte) error {
	if len(data) == 0 {
		return errInvalidOpcodeData
	}
	switch data[0] {
	case SigSchemeSecp256k1:
		return pushData(data, 66, c)
	default:
		return errInvalidSigScheme
	}
}

// opPushPubKey pushes pubKey to the stack. The number of bytes of data to read is determined by first byte <SigScheme> label:
// 0x01 – <secp256k1>, 33 bytes
func opPushPubKey(c *scriptContext, data []byte) error {
	if len(data) == 0 {
		return errInvalidOpcodeData
	}
	switch data[0] {
	case SigSchemeSecp256k1:
		return pushData(data, 34, c)
	default:
		return errInvalidSigScheme
	}
}

// opPushHash pushes a hash value to the stack. The number of bytes of data to read is determined by first byte <HashAlg> label:
// 0x01 – SHA256, 32 bytes
// 0x02 – SHA512, 64 bytes
func opPushHash(c *scriptContext, data []byte) error {
	if len(data) == 0 {
		return errInvalidOpcodeData
	}
	switch data[0] {
	case HashAlgSha256:
		return pushData(data, 33, c)
	case HashAlgSha512:
		return pushData(data, 65, c)
	default:
		return errInvalidHashAlgo
	}
}

func pushData(data []byte, dataLength int, c *scriptContext) error {
	if len(data) == dataLength {
		c.stack.push(data[1:])
		return nil
	}
	return errInvalidOpcodeData
}

// opDup duplicates top element on the stack, returns error if stack is empty
func opDup(c *scriptContext, data []byte) error {
	if len(data) != 0 {
		return errInvalidOpcodeData
	}
	peek, err := c.stack.peek()
	if err != nil {
		return err
	}
	c.stack.push(peek)
	return nil
}

// opEqual removes two top elements, compares them for equality and push the resulting bool to the stack.
func opEqual(c *scriptContext, data []byte) error {
	if len(data) != 0 {
		return errInvalidOpcodeData
	}
	a, err := c.stack.pop()
	if err != nil {
		return err
	}
	b, err := c.stack.pop()
	if err != nil {
		return err
	}
	c.stack.pushBool(bytes.Equal(a, b))
	return nil
}

// opVerify checks if top of the stack is bool TRUE and removes it from the stack.
func opVerify(c *scriptContext, data []byte) error {
	if len(data) != 0 {
		return errInvalidOpcodeData
	}
	pop, err := c.stack.popBool()
	if err != nil {
		return err
	}
	if !pop {
		return errInvalidOpcodeData
	}
	return nil
}

// opHash hashes the top value on the stack using the hash algorithm specified in the byte following the opcode.
// HashAlgs:
// 0x01 – SHA256
// 0x02 – SHA512
func opHash(c *scriptContext, data []byte) error {
	pop, err := c.stack.pop()
	if err != nil {
		return err
	}
	if len(data) != 1 {
		return errInvalidOpcodeData
	}
	switch data[0] {
	case HashAlgSha256:
		c.stack.push(hash.Sum256(pop))
		return nil
	case HashAlgSha512:
		c.stack.push(hash.Sum512(pop))
		return nil
	default:
		return errInvalidHashAlgo
	}
}

// opCheckSig verifies that top of the stack contains pubKey and signature that were used to sign sigData
// Returns either error or pushes TRUE/FALSE to the stack indicating signature verification result
func opCheckSig(c *scriptContext, data []byte) error {
	if len(data) != 1 {
		return errInvalidOpcodeData
	}
	if data[0] != SigSchemeSecp256k1 {
		return errInvalidSigScheme
	}
	pubKey, err := c.stack.pop()
	if err != nil {
		return err
	}
	sig, err := c.stack.pop()
	if err != nil {
		return err
	}

	verifier, err := crypto.NewVerifierSecp256k1(pubKey)
	if err != nil {
		return err
	}
	err = verifier.VerifyBytes(sig, c.sigData)
	c.stack.pushBool(err == nil)
	return nil
}
