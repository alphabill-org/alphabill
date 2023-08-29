package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
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

// Message implements ethereum core.Message
type Message struct {
	to         *common.Address
	from       common.Address
	nonce      uint64
	amount     *big.Int
	gasLimit   uint64
	gasPrice   *big.Int
	gasFeeCap  *big.Int
	gasTipCap  *big.Int
	data       []byte
	accessList ethtypes.AccessList
	isFake     bool
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
func (t *TxAttributes) AsMessage(gasPrice *big.Int) Message {
	msg := Message{
		nonce:      t.Nonce,
		gasLimit:   t.Gas,
		gasPrice:   gasPrice,
		gasFeeCap:  big.NewInt(1),
		gasTipCap:  big.NewInt(0),
		to:         t.ToAddr(),
		from:       t.FromAddr(),
		amount:     t.Value,
		data:       t.Data,
		accessList: ethtypes.AccessList{},
		isFake:     false,
	}
	return msg
}

func (m Message) From() common.Address            { return m.from }
func (m Message) To() *common.Address             { return m.to }
func (m Message) GasPrice() *big.Int              { return m.gasPrice }
func (m Message) GasFeeCap() *big.Int             { return m.gasFeeCap }
func (m Message) GasTipCap() *big.Int             { return m.gasTipCap }
func (m Message) Value() *big.Int                 { return m.amount }
func (m Message) Gas() uint64                     { return m.gasLimit }
func (m Message) Nonce() uint64                   { return m.nonce }
func (m Message) Data() []byte                    { return m.data }
func (m Message) AccessList() ethtypes.AccessList { return m.accessList }
func (m Message) IsFake() bool                    { return m.isFake }
