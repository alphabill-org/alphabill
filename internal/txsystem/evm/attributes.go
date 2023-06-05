package evm

import "math/big"

const PayloadTypeEVMCall = "evm"

type TxAttributes struct {
	_        struct{} `cbor:",toarray"`
	From     []byte
	To       []byte
	Data     []byte
	Value    *big.Int
	Gas      uint64
	GasPrice *big.Int
}
