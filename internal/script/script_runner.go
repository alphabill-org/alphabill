package _s

import (
	"errors"
	"fmt"
)

var _ = Register(&PredicateScriptRunnerFactory{})

const MaxScriptBytes = 65536
const StartByte = 0x53

// RunScript performs initial validation, selects the correct script runner and calls execute.
func RunScript(predicateArgument []byte, bearerPredicate []byte, sigData []byte) error {
	if err := validatePredicateSize(bearerPredicate); err != nil {
		return fmt.Errorf("invalid script format: %w", err)
	}

	runner, err := FindRunner(bearerPredicate)
	if err != nil {
		return err
	}

	return runner.Execute(predicateArgument, sigData)
}

type scriptContext struct {
	stack   *stack
	sigData []byte // canonically serialized unhashed signature data
}

func executeScript(script []byte, sc *scriptContext) error {
	for i := 1; i < len(script); i++ { // i is always incremented at least once per opcode
		op, exists := opCodes[script[i]]
		if !exists {
			return fmt.Errorf("unknown opcode 0x%x", script[i])
		}

		dataLength, err := op.getDataLength(script[i+1:])
		if err != nil {
			return fmt.Errorf("failed to get data length for opcode 0x%x: %w", op.value, err)
		}

		err = op.exec(sc, script[i+1:i+1+dataLength])
		if err != nil {
			return fmt.Errorf("failed to execute opcode 0x%x: %w", op.value, err)
		}

		i += dataLength
	}
	return nil
}

func validateInput(predicateArgument, bearerPredicate []byte) error {
	if err := validatePredicate(predicateArgument); err != nil {
		return fmt.Errorf("predicate argument is invalid: %w", err)
	}
	if err := validatePredicate(bearerPredicate); err != nil {
		return fmt.Errorf("bearer predicate is invalid: %w", err)
	}
	return nil
}

func validatePredicate(predicate []byte) error {
	if len(predicate) == 0 {
		return errors.New("predicate is empty")
	}
	if len(predicate) > MaxScriptBytes {
		return errors.New("predicate is too large")
	}
	if predicate[0] != StartByte {
		return errors.New("predicate does not start with StartByte")
	}
	return nil
}

func validatePredicateSize(predicate []byte) error {
	if len(predicate) == 0 {
		return errors.New("predicate is empty")
	}
	if len(predicate) > MaxScriptBytes {
		return errors.New("predicate is too large")
	}
	return nil
}

func selectRunner(predicate []byte) (PredicateRunner, error) {
	if predicate[0] == StartByte {
		return newPredicateScriptRunner(predicate)
	}
	return nil, errors.New("no predicate runner found")
}

type predicateScriptRunner struct {
	predicate []byte
}

func newPredicateScriptRunner(predicate []byte) (*predicateScriptRunner, error) {
	if predicate[0] != StartByte {
		return nil, errors.New("predicate does not start with 0x53")
	}
	return &predicateScriptRunner{
		predicate: predicate,
	}, nil
}

func (p *predicateScriptRunner) validateSignature(sig []byte) error {
	if sig[0] != StartByte {
		return errors.New("predicate does not start with StartByte")
	}
	return nil
}

/*
Execute executes the given script. If the script contains OpCheckSig opCode then correct sigData must be supplied.
The script is considered valid if after execution there's only one TRUE value on the stack, otherwise error is returned.

An example P2PKH script format:
BearerPredicate:   [Dup, Hash <SHA256>, PushHash <SHA256> <32 bytes>, Equal, Verify, CheckSig <secp256k1>]
PredicateArgument: [PushSig <secp256k1> <65 bytes>, PushPubKey <secp256k1> <33 bytes>]

Same example with byte encoding
BearerPredicate:   [0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01, <32 bytes>, 0x87, 0x69, 0xac, 0x01]
PredicateArgument: [0x53, 0x54, 0x01, <65 bytes>, 0x55, 0x01, <33 bytes>]
*/
func (p *predicateScriptRunner) Execute(sig []byte, sigData []byte) error {
	if err := p.validateSignature(sig); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	sc := &scriptContext{
		stack:   &stack{},
		sigData: sigData,
	}

	err := executeScript(sig, sc)
	if err != nil {
		return fmt.Errorf("predicate argument execution failed: %w", err)
	}
	err = executeScript(p.predicate, sc)
	if err != nil {
		return fmt.Errorf("bearer predicate execution failed: %w", err)
	}

	res, err := sc.stack.popBool()
	if err != nil {
		return fmt.Errorf("failed to pop top of the stack: %w", err)
	}
	if !res {
		return errors.New("script execution result yielded false")
	}
	if !sc.stack.isEmpty() {
		return errors.New("script execution result yielded non-clean stack")
	}
	return nil
}

type PredicateScriptRunnerFactory struct {
}

func (f *PredicateScriptRunnerFactory) IsApplicable(predicate []byte) bool {
	return predicate[0] == StartByte
}

func (f *PredicateScriptRunnerFactory) Create(predicate []byte) (PredicateRunner, error) {
	return newPredicateScriptRunner(predicate)
}
