package unit

import (
	"bytes"
	"fmt"
	"hash"
	"math/big"

	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm/conversion"
	fc "github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fxamacker/cbor/v2"
)

var (
	_             abstate.UnitData          = (*StateObject)(nil)
	_             fc.GenericFeeCreditRecord = (*StateObject)(nil)
	EmptyCodeHash                           = crypto.Keccak256(nil)
)

type StateObject struct {
	_       struct{} `cbor:",toarray"`
	Account *Account
	Storage state.Storage

	Suicided bool
}

// Account represents an account in Ethereum.
type Account struct {
	_        struct{} `cbor:",toarray"`
	Balance  *big.Int
	CodeHash []byte
	Code     []byte
	Nonce    uint64
	Backlink []byte // for account to functions as Alphabill fee credit record (to be replaced with nonce in the future)
	Timeout  uint64 // for account to functions as Alphabill fee credit record (see FCR spec for details)
	Locked   uint64 // for account to functions as Alphabill fee credit record
}

func NewEvmFcr(v uint64, backlink []byte, timeout uint64) fc.GenericFeeCreditRecord {
	return &StateObject{
		Account: &Account{
			Nonce:    0,
			Balance:  conversion.AlphaToWei(v),
			CodeHash: EmptyCodeHash,
			Backlink: backlink,
			Timeout:  timeout,
		},
		Storage: map[common.Hash]common.Hash{},
	}
}

func NewStateObject() *StateObject {
	return &StateObject{
		Account: &Account{
			Nonce:    0,
			Balance:  big.NewInt(0),
			CodeHash: EmptyCodeHash,
		},
		Storage: map[common.Hash]common.Hash{},
	}
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
		Account:  s.Account.Copy(),
		Storage:  s.Storage.Copy(),
		Suicided: s.Suicided,
	}
}

func (a *Account) Copy() *Account {
	return &Account{
		Balance:  big.NewInt(0).SetBytes(bytes.Clone(a.Balance.Bytes())),
		CodeHash: bytes.Clone(a.CodeHash),
		Code:     bytes.Clone(a.Code),
		Nonce:    a.Nonce,
		Backlink: bytes.Clone(a.Backlink),
		Timeout:  a.Timeout,
		Locked:   a.Locked,
	}
}

func (s *StateObject) Empty() bool {
	return s.Account.Nonce == 0 && s.Account.Balance.Sign() == 0 && bytes.Equal(s.Account.CodeHash, EmptyCodeHash)
}

func (s *StateObject) AddCredit(v uint64, backlink []byte, timeout uint64) {
	s.Account.Balance = new(big.Int).Add(s.Account.Balance, conversion.AlphaToWei(v))
	s.Account.Backlink = bytes.Clone(backlink)
	s.Account.Timeout = timeout
	s.Account.Locked = 0
}

func (s *StateObject) DecCredit(v uint64, backlink []byte) {
	s.Account.Balance = new(big.Int).Sub(s.Account.Balance, conversion.AlphaToWei(v))
	if backlink != nil {
		s.Account.Backlink = bytes.Clone(backlink)
	}
}

// GetBalance - Fee credit record interface-method returns balance as "tema"
// NB! fractions are rounded down 1.9 "tema" is still 1 "tema"
func (s *StateObject) GetBalance() uint64 {
	// round down
	return conversion.WeiToAlphaRoundDown(s.Account.Balance)
}

func (s *StateObject) GetTimeout() uint64 {
	return s.Account.Timeout
}

func (s *StateObject) GetBacklink() []byte {
	if s == nil {
		return nil
	}
	return s.Account.Backlink
}

func (s *StateObject) UpdateLock(lock uint64, fee uint64, backlink []byte) {
	s.Account.Locked = lock
	s.Account.Balance = new(big.Int).Sub(s.Account.Balance, conversion.AlphaToWei(fee))
	s.Account.Backlink = bytes.Clone(backlink)
}

func (s *StateObject) IsLocked() bool {
	return s.Account.Locked != 0
}
