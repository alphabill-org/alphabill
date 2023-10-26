package client

import (
	"math/big"

	"github.com/alphabill-org/alphabill/validator/internal/txsystem/evm/statedb"
	"github.com/ethereum/go-ethereum/common"
)

type TxAttributes struct {
	_     struct{} `cbor:",toarray"`
	From  []byte
	To    []byte
	Data  []byte
	Value *big.Int
	Gas   uint64
	Nonce uint64
}

type CallAttributes struct {
	_     struct{} `cbor:",toarray"`
	From  []byte
	To    []byte
	Data  []byte
	Value *big.Int
	Gas   uint64
}

type ProcessingDetails struct {
	_            struct{} `cbor:",toarray"`
	ErrorDetails string
	ReturnData   []byte
	ContractAddr common.Address
	Logs         []*statedb.LogEntry
}

type Result struct {
	Success   bool
	ActualFee uint64
	Details   *ProcessingDetails
}
