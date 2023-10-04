package account

import (
	"sync"
)

type (
	// accounts helper struct caching Account public keys
	accounts struct {
		mu       sync.Mutex // mu mutex guarding accounts field
		accounts []Account
	}
	Account struct {
		AccountIndex uint64
		AccountKeys  KeyHashes
	}
)

func NewAccount(idx uint64, key AccountKey) *Account {
	return &Account{
		AccountIndex: idx,
		AccountKeys:  *key.PubKeyHash,
	}
}

func (a *accounts) add(account *Account) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.accounts = append(a.accounts, *account)
}

func (a *accounts) getAll() []Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accounts
}
