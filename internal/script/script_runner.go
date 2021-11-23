package script

import "errors"

const MaxScriptBytes = 65536
const StartByte = 0x53

type scriptContext struct {
	stack   *stack
	sigData []byte // canonically serialized unhashed signature data
}

var (
	errUnknownOpCode       = errors.New("unknown opcode")
	errInvalidScriptFormat = errors.New("invalid script format")
	errScriptResultFalse   = errors.New("script execution result yielded false or non-clean stack")
)

/*
RunScript executes the given script. If the script contains OpCheckSig opCode then correct sigData must be supplied.
It's possible for the script to return FALSE with no error.

An example P2PKH script format:
BearerPredicate:   [Dup, Hash <SHA256>, PushHash <SHA256> <32 bytes>, Equal, Verify, CheckSig <secp256k1>]
PredicateArgument: [PushSig <secp256k1> <65 bytes>, PushPubKey <secp256k1> <33 bytes>]

Same example with byte encoding
BearerPredicate:   [0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01, <32 bytes>, 0x87, 0x69, 0xac, 0x01]
PredicateArgument: [0x53, 0x54, 0x01, <65 bytes>, 0x55, 0x01, <33 bytes>]
*/
func RunScript(predicateArgument []byte, bearerPredicate []byte, sigData []byte) error {
	if !validateInput(predicateArgument, bearerPredicate) {
		return errInvalidScriptFormat
	}

	sc := scriptContext{
		stack:   &stack{},
		sigData: sigData,
	}

	err := executeScript(predicateArgument, &sc)
	if err != nil {
		return err
	}
	err = executeScript(bearerPredicate, &sc)
	if err != nil {
		return err
	}

	top, err := sc.stack.popBool()
	if err != nil {
		return err
	}
	if top && sc.stack.isEmpty() {
		return nil
	}
	return errScriptResultFalse
}

func executeScript(script []byte, sc *scriptContext) error {
	for i := 1; i < len(script); i++ { // i is always incremented at least once per opcode
		op, exists := opCodes[script[i]]
		if !exists {
			return errUnknownOpCode
		}

		dataLength, err := op.getDataLength(script[i+1:])
		if err != nil {
			return err
		}

		err = op.exec(sc, script[i+1:i+1+dataLength])
		if err != nil {
			return err
		}

		i += dataLength
	}
	return nil
}

func validateInput(predicate []byte, signature []byte) bool {
	if len(signature) > MaxScriptBytes || len(predicate) > MaxScriptBytes {
		return false
	}
	if len(signature) == 0 || len(predicate) == 0 {
		return false
	}
	if signature[0] != StartByte || predicate[0] != StartByte {
		return false
	}
	return true
}
