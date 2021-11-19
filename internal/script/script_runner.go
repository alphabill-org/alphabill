package script

const MaxScriptBytes = 65536
const StartByte = 0x53

type context struct {
	stack   *stack
	sigData []byte // canonically serialized unhashed signature data
}

/*
Executes given script. If the script contains OP_CHECKSIG opCode then correct sigData must be supplied.

An example P2PKH script format:
BearerPredicate:   [Dup, Hash <SHA256>, PushHash <SHA256> <32 bytes>, Equal, Verify, CheckSig <secp256k1>]
PredicateArgument: [PushSig <secp256k1> <65 bytes>, PushPubKey <secp256k1> <33 bytes>]

Same example with byte encoding
BearerPredicate:   [0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01, <32 bytes>, 0x87, 0x69, 0xac, 0x01]
PredicateArgument: [0x53, 0x54, 0x01, <65 bytes>, 0x55, 0x01, <33 bytes>]
*/
func RunScript(predicateArgument []byte, bearerPredicate []byte, sigData []byte) bool {
	if !validateInput(predicateArgument, bearerPredicate) {
		return false
	}

	scriptContext := context{
		stack:   &stack{},
		sigData: sigData,
	}

	if !executeScript(predicateArgument, &scriptContext) {
		return false
	}
	if !executeScript(bearerPredicate, &scriptContext) {
		return false
	}

	top, _ := scriptContext.stack.popBool()
	return top && scriptContext.stack.isEmpty()
}

func executeScript(script []byte, scriptContext *context) bool {
	for i := 1; i < len(script); i++ { // i always incremented at least once per opcode
		opCode, exists := opCodes[script[i]]
		if !exists {
			return false
		}

		dataLength, err := opCode.getDataLength(opCode, script[i+1:])
		if err != nil {
			return false
		}

		err = opCode.exec(scriptContext, script[i+1:i+1+dataLength])
		if err != nil {
			return false
		}

		if dataLength > 0 {
			i += dataLength
		}
	}
	return true
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
