package statedb

import (
	"bytes"
	"hash"
	"math/big"
	"sort"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
)

var emptyCodeHash = crypto.Keccak256(nil)

// Account represents an account in Ethereum.
type Account struct {
	Balance  *big.Int
	CodeHash []byte
	Code     []byte
	Nonce    uint64
}

type FeeBillData struct {
	Bearer  []byte
	UnitID  []byte
	TxHash  []byte
	Timeout uint64
}

type StateObject struct {
	Address   common.Address
	Account   *Account
	Storage   state.Storage
	dirtyCode bool
	suicided  bool
	FeeBill   *FeeBillData
}

func (a *Account) Write(hasher hash.Hash) {
	hasher.Write(a.Balance.Bytes())
	hasher.Write(a.CodeHash)
	hasher.Write(a.Code)
	hasher.Write(util.Uint64ToBytes(a.Nonce))
}

func (f *FeeBillData) AddToHasher(hasher hash.Hash) {
	hasher.Write(f.Bearer)
	hasher.Write(f.UnitID)
	hasher.Write(f.TxHash)
	hasher.Write(util.Uint64ToBytes(f.Timeout))
}

func (s *StateObject) AddToHasher(hasher hash.Hash) {
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
	if s.FeeBill != nil {
		s.FeeBill.AddToHasher(hasher)
	}
}

func (s *StateObject) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}

func (s *StateObject) empty() bool {
	return s.Account.Nonce == 0 && s.Account.Balance.Sign() == 0 && bytes.Equal(s.Account.CodeHash, emptyCodeHash)
}
