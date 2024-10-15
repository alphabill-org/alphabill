package statedb

import (
	"bytes"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/holiman/uint256"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	_ types.UnitData = (*StateObject)(nil)

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
	Balance  *uint256.Int
	CodeHash []byte
	Code     []byte
	Nonce    uint64
}

// AlphaBillLink links Account to AB FCR bill
type AlphaBillLink struct {
	_              struct{} `cbor:",toarray"`
	Counter        uint64
	Timeout        uint64
	OwnerPredicate []byte
}

func (s *StateObject) Write(hasher hash.Hash) error {
	res, err := types.Cbor.Marshal(s)
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

func (s *StateObject) Copy() types.UnitData {
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

func (s *StateObject) Owner() []byte {
	if s == nil || s.AlphaBill == nil {
		return nil
	}
	return s.AlphaBill.OwnerPredicate
}

func (f *AlphaBillLink) Copy() *AlphaBillLink {
	if f == nil {
		return nil
	}
	return &AlphaBillLink{
		Counter:        f.Counter,
		Timeout:        f.Timeout,
		OwnerPredicate: bytes.Clone(f.OwnerPredicate),
	}
}

func (f *AlphaBillLink) GetTimeout() uint64 {
	if f != nil {
		return f.Timeout
	}
	return 0
}

func (f *AlphaBillLink) GetCounter() uint64 {
	if f != nil {
		return f.Counter
	}
	return 0
}

func (a *Account) Copy() *Account {
	return &Account{
		Balance:  a.Balance.Clone(),
		CodeHash: bytes.Clone(a.CodeHash),
		Code:     bytes.Clone(a.Code),
		Nonce:    a.Nonce,
	}
}

func (s *StateObject) empty() bool {
	return s.Account.Nonce == 0 && s.Account.Balance.Sign() == 0 && bytes.Equal(s.Account.CodeHash, emptyCodeHash)
}
