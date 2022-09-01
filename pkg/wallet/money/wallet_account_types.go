package money

import (
	"sync"

	"github.com/alphabill-org/alphabill/pkg/wallet"
)

type (
	// accounts helper struct caching account public keys
	accounts struct {
		mu       sync.Mutex // mu mutex guarding accounts field
		accounts []account
	}
	account struct {
		accountIndex uint64
		accountKeys  wallet.KeyHashes
	}
)

func newAccountsCache() *accounts {
	return &accounts{accounts: make([]account, 0)}
}

func (a *accounts) add(account *account) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.accounts = append(a.accounts, *account)
}

func (a *accounts) getAll() []account {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accounts
}
