package statedb

import (
	"bytes"
	"fmt"
	"hash"
	"math/big"

	abstate "github.com/alphabill-org/alphabill/internal/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fxamacker/cbor/v2"
)

var (
	_ abstate.UnitData = (*StateObject)(nil)

	emptyCodeHash = crypto.Keccak256(nil)
)

type StateObject struct {
	_         struct{} `cbor:",toarray"`
	Address   common.Address
	Account   *Account
	Storage   state.Storage
	AlphaBill *AlphaBillLink

	Suicided bool
}

// Account represents an account in Ethereum.
type Account struct {
	_        struct{} `cbor:",toarray"`
	Balance  *big.Int
	CodeHash []byte
	Code     []byte
	Nonce    uint64
}

// AlphaBillLink links Account to AB FCR bill
type AlphaBillLink struct {
	_       struct{} `cbor:",toarray"`
	TxHash  []byte
	Timeout uint64
}

func (s *StateObject) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(s)
	if err != nil {
		return fmt.Errorf("unit data encode error: %w", err)
	}
	// s.Storage is map which will be serialized in sorted order by CBOR
	// however when deserializing we will not get back the same map (order will be different)
	// if this becomes an issue then map cannot be used, or it needs special serializaion
	_, err = hasher.Write(res)
	return err
}

func (s *StateObject) SummaryValueInput() uint64 {
	return 0
}

func (s *StateObject) Copy() abstate.UnitData {
	if s == nil {
		return nil
	}

	return &StateObject{
		Address:   common.BytesToAddress(bytes.Clone(s.Address.Bytes())),
		Account:   s.Account.Copy(),
		Storage:   s.Storage.Copy(),
		AlphaBill: s.AlphaBill.Copy(),
		Suicided:  s.Suicided,
	}
}

func (f *AlphaBillLink) Copy() *AlphaBillLink {
	if f == nil {
		return nil
	}
	return &AlphaBillLink{
		TxHash:  bytes.Clone(f.TxHash),
		Timeout: f.Timeout,
	}
}

func (f *AlphaBillLink) GetTimeout() uint64 {
	if f != nil {
		return f.Timeout
	}
	return 0
}

func (a *Account) Copy() *Account {
	return &Account{
		Balance:  big.NewInt(0).SetBytes(bytes.Clone(a.Balance.Bytes())),
		CodeHash: bytes.Clone(a.CodeHash),
		Code:     bytes.Clone(a.Code),
		Nonce:    a.Nonce,
	}
}

func (s *StateObject) empty() bool {
	return s.Account.Nonce == 0 && s.Account.Balance.Sign() == 0 && bytes.Equal(s.Account.CodeHash, emptyCodeHash)
}
