package script

import (
	"bytes"
	"errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
)

type opCode struct {
	value         byte
	dataLength    int
	exec          func(*context, []byte) error
	getDataLength func(opCode, []byte) (int, error)
}

const (
	OP_DUP          = 0x76
	OP_HASH         = 0xa8
	OP_PUSH_HASH    = 0x4f
	OP_PUSH_PUB_KEY = 0x55
	OP_PUSH_SIG     = 0x54
	OP_CHECKSIG     = 0xac
	OP_EQUAL        = 0x87
	OP_VERIFY       = 0x69
	OP_PUSH_BOOL    = 0x51

	// TODO implement below opcodes
	//OP_PUSH_INT64   = 0x01
	//OP_IF           = 0x63
	//OP_ELSE         = 0x67
	//OP_END_IF       = 0x68
	//OP_SWAP         = 0x7c
	//OP_DROP         = 0x75
	//OP_NOT          = 0x91
)

var opCodes = map[byte]opCode{
	OP_PUSH_BOOL:    {OP_PUSH_BOOL, 1, opPushBool, getDataLength},
	OP_PUSH_SIG:     {OP_PUSH_SIG, 66, opPushSig, getDataLength},
	OP_PUSH_PUB_KEY: {OP_PUSH_PUB_KEY, 34, opPushPubKey, getDataLength},
	OP_PUSH_HASH:    {OP_PUSH_HASH, -1, opPushHash, getPushHashDataLength},
	OP_HASH:         {OP_HASH, 1, opHash, getDataLength},
	OP_DUP:          {OP_DUP, 0, opDup, getDataLength},
	OP_EQUAL:        {OP_EQUAL, 0, opEqual, getDataLength},
	OP_VERIFY:       {OP_VERIFY, 0, opVerify, getDataLength},
	OP_CHECKSIG:     {OP_CHECKSIG, 1, opCheckSig, getDataLength},
}

var (
	errInvalidOpcodeData = errors.New("invalid opcode data")
)

// Returns fixed opCode dataLength or error if data is out of bounds from script
func getDataLength(opCode opCode, script []byte) (int, error) {
	if len(script) < opCode.dataLength {
		return 0, errInvalidOpcodeData
	}
	return opCode.dataLength, nil
}

// Returns parsed data length of OP_PUSH_HASH
func getPushHashDataLength(code opCode, script []byte) (int, error) {
	if len(script) == 0 {
		return 0, errInvalidOpcodeData
	}
	lenByte := script[0]
	if lenByte == 0x01 && len(script) >= 33 {
		return 33, nil
	}
	if lenByte == 0x02 && len(script) >= 65 {
		return 65, nil
	}
	return 0, errInvalidOpcodeData
}

// Push bool to stack, returns error if data is not a valid bool
func opPushBool(c *context, data []byte) error {
	if len(data) == 1 && (data[0] == 0x00 || data[0] == 0x01) {
		c.stack.push(data)
		return nil
	}
	return errInvalidOpcodeData
}

// Push sig to the stack. The number of bytes of data to read is determined by first byte <SigScheme> label:
// 0x01 – <secp256k1>, 65 bytes
func opPushSig(c *context, data []byte) error {
	if len(data) == 66 && data[0] == 0x01 {
		c.stack.push(data[1:])
		return nil
	}
	return errInvalidOpcodeData
}

// Push pubKey to the stack. The number of bytes of data to read is determined by first byte <SigScheme> label:
// 0x01 – <secp256k1>, 33 bytes
func opPushPubKey(c *context, data []byte) error {
	if len(data) == 34 && data[0] == 0x01 {
		c.stack.push(data[1:])
		return nil
	}
	return errInvalidOpcodeData
}

// Push a hash value to the stack. The number of bytes of data to read is determined by first byte <HashAlg> label:
// 0x01 – SHA256, 32 bytes
// 0x02 – SHA512, 64 bytes
func opPushHash(c *context, data []byte) error {
	if len(data) == 33 && data[0] == 0x01 {
		c.stack.push(data[1:])
		return nil
	}
	if len(data) == 65 && data[0] == 0x02 {
		c.stack.push(data[1:])
		return nil
	}
	return errInvalidOpcodeData
}

// Duplicate the value at the top of the stack, returns error if stack is empty
func opDup(c *context, data []byte) error {
	peek, err := c.stack.peek()
	if err != nil {
		return err
	}
	c.stack.push(peek)
	return nil
}

// Removes two top elements, compares them for equality and push the resulting bool to the stack.
func opEqual(c *context, data []byte) error {
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

// Checks if top of the stack is bool TRUE and removes it from the stack.
func opVerify(c *context, data []byte) error {
	pop, err := c.stack.popBool()
	if err != nil {
		return err
	}
	if !pop {
		return errors.New("expected top value to be true but was false")
	}
	return nil
}

// Hash the value at the top of the stack using the hash algorithm specified in the byte following the opcode. HashAlgs:
// 0x01 – SHA256
// 0x02 – SHA512
func opHash(c *context, data []byte) error {
	pop, err := c.stack.pop()
	if err != nil {
		return err
	}
	if len(data) != 1 {
		return errInvalidOpcodeData
	}
	if data[0] == 0x01 {
		c.stack.push(hash.Sum256(pop))
		return nil
	}
	if data[0] == 0x02 {
		c.stack.push(hash.Sum512(pop))
		return nil
	}
	return errInvalidOpcodeData
}

// Verifies that top of the stack contains pubKey and signature that were used to sign sigData
// Returns either error or pushes TRUE/FALSE to the stack indicating signature verification result
func opCheckSig(c *context, data []byte) error {
	if len(data) != 1 || data[0] != 0x01 { // <secp256k1> SigScheme
		return errInvalidOpcodeData
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
	if err != nil {
		c.stack.pushBool(false)
	} else {
		c.stack.pushBool(true)
	}
	return nil
}
