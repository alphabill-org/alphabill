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

	// Shutdown terminates connection to alphabill node and closes wallet db
	Shutdown()
}

// NewWallet creates a new in memory wallet. To synchronize wallet with node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func NewWallet() (Wallet, error) {
	return wallet.NewWallet()
}
