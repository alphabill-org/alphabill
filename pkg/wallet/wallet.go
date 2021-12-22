package wallet

import (
	"alphabill-wallet-sdk/internal/wallet"
	"alphabill-wallet-sdk/pkg/wallet/config"
)

type Wallet interface {
	GetBalance() uint64
	Send(pubKey []byte, amount uint64) error

	// Sync synchronises wallet with given alphabill node
	Sync(conf *config.AlphaBillClientConfig) error

	// Shutdown terminates connection to alphabill node
	Shutdown()
}

// NewInMemoryWallet creates a new in memory wallet. To synchronize wallet with node call Sync
func NewInMemoryWallet() (Wallet, error) {
	return wallet.NewInMemoryWallet()
}
