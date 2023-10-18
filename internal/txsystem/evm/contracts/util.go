package contracts

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

func PackOutput(a abi.ABI, name string, args ...interface{}) ([]byte, error) {
	method, exist := a.Methods[name]
	if !exist {
		return nil, fmt.Errorf("function '%s' not found", name)
	}
	arguments, err := method.Outputs.Pack(args...)
	if err != nil {
		return nil, err
	}
	return arguments, nil
}

func UnpackInput(a abi.ABI, name string, data []byte) ([]interface{}, error) {
	if len(data)%32 != 0 {
		return nil, fmt.Errorf("abi: improperly formatted input: %s - Bytes: [%+v]", string(data), data)
	}
	args, err := getMethodInputs(a, name)
	if err != nil {
		return nil, err
	}
	return args.Unpack(data)
}

func getMethodInputs(a abi.ABI, name string) (abi.Arguments, error) {
	var args abi.Arguments
	if method, ok := a.Methods[name]; ok {
		args = method.Inputs
	}
	if args == nil {
		return nil, fmt.Errorf("abi: could not locate named function: %s", name)
	}
	return args, nil
}
