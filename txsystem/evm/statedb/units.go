package statedb

import (
	"bytes"
	"hash"
	"math/big"
	"sort"

	"github.com/alphabill-org/alphabill/common/util"
	abstate "github.com/alphabill-org/alphabill/txsystem/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	_ abstate.UnitData = (*StateObject)(nil)

	emptyCodeHash = crypto.Keccak256(nil)
)

type StateObject struct {
	Address   common.Address
	Account   *Account
	Storage   state.Storage
	AlphaBill *AlphaBillLink

	suicided bool
}

// Account represents an account in Ethereum.
type Account struct {
	Balance  *big.Int
	CodeHash []byte
	Code     []byte
	Nonce    uint64
}

// AlphaBillLink links Account to AB FCR bill
type AlphaBillLink struct {
	TxHash  []byte
	Timeout uint64
}

func (s *StateObject) Write(hasher hash.Hash) {
	hasher.Write(s.Address.Bytes())
	s.Account.Write(hasher)
	keys := make([]common.Hash, 0, len(s.Storage))

	for key := range s.Storage {
		keys = append(keys, key)
	}

	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i].Big().Cmp(keys[j].Big()) > 0
	})

	for _, k := range keys {
		hasher.Write(k.Bytes())
		hasher.Write(s.Storage[k].Bytes())
	}
	if s.AlphaBill != nil {
		s.AlphaBill.Write(hasher)
	}
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
		suicided:  s.suicided,
	}
}

func (f *AlphaBillLink) Write(hasher hash.Hash) {
	hasher.Write(f.TxHash)
	hasher.Write(util.Uint64ToBytes(f.Timeout))
}

func (f *AlphaBillLink) SummaryValueInput() uint64 {
	return 0
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

func (a *Account) Write(hasher hash.Hash) {
	hasher.Write(a.Balance.Bytes())
	hasher.Write(a.CodeHash)
	hasher.Write(a.Code)
	hasher.Write(util.Uint64ToBytes(a.Nonce))
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
