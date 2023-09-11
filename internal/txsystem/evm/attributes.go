package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

const PayloadTypeEVMCall = "evm"

type TxAttributes struct {
	_     struct{} `cbor:",toarray"`
	From  []byte
	To    []byte
	Data  []byte
	Value *big.Int
	Gas   uint64
	Nonce uint64
}

// FromAddr - returns From as Address, if nil empty address is returned
// From is mandatory field and must not be nil in a valid TxAttributes
func (t *TxAttributes) FromAddr() common.Address {
	if t == nil || t.From == nil {
		return common.Address{}
	}
	return common.BytesToAddress(t.From)
}

// ToAddr - returns To as Address pointer.
// To field is optional and not present on contract creation calls.
// If To is nil then nil pointer is returned
func (t *TxAttributes) ToAddr() *common.Address {
	if t == nil || t.To == nil {
		return nil
	}
	addr := common.BytesToAddress(t.To)
	return &addr
}

// AsMessage returns the Alphabill transaction as a ethereum core.Message.
func (t *TxAttributes) AsMessage(gasPrice *big.Int, fake bool) *core.Message {
	msg := &core.Message{
		Nonce:             t.Nonce,
		GasLimit:          t.Gas,
		GasPrice:          gasPrice,
		GasFeeCap:         gasPrice,      // gas price is constant, meaning max fee is the same as gas unit price
		GasTipCap:         big.NewInt(0), // only used in London system, no supported for AB
		To:                t.ToAddr(),
		From:              t.FromAddr(),
		Value:             t.Value,
		Data:              t.Data,
		AccessList:        ethtypes.AccessList{},
		BlobGasFeeCap:     nil, // todo: investigate
		BlobHashes:        nil, // todo: investigate
		SkipAccountChecks: fake,
	}
	return msg
}
