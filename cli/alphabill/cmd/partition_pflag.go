package cmd

import (
	"errors"
)

// partitionType "partition" cli flag, implements github.com/spf13/pflag/flag.go#Value interface
type partitionType string

const (
	moneyType  partitionType = "money"
	tokensType partitionType = "tokens"
	evmType    partitionType = "evm"
)

// String returns string value of given partitionType, used in Printf and help context
func (e *partitionType) String() string {
	return string(*e)
}

// Set sets the value of this partitionType string
func (e *partitionType) Set(v string) error {
	switch v {
	case "money", "tokens", "evm":
		*e = partitionType(v)
		return nil
	default:
		return errors.New("must be one of [money|tokens|evm]")
	}
}

// Type used to show the type value in the help context
func (e *partitionType) Type() string {
	return "string"
}
