package account

import (
	"sync"

	"github.com/alphabill-org/alphabill/pkg/wallet"
)

type (
	// accounts helper struct caching Account public keys
	accounts struct {
		mu       sync.Mutex // mu mutex guarding accounts field
		accounts []Account
	}
	Account struct {
		AccountIndex uint64
		AccountKeys  wallet.KeyHashes
	}
)

func newAccountsCache() *accounts {
	return &accounts{accounts: make([]Account, 0)}
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
