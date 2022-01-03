package wallet

import (
	"alphabill-wallet-sdk/internal/wallet"
	"alphabill-wallet-sdk/pkg/wallet/config"
)

type Wallet interface {
	GetBalance() (uint64, error)
	Send(pubKey []byte, amount uint64) error

	// Sync synchronises wallet with given alphabill node, blocks forever or until alphabill connection is terminated
	Sync() error

	// Shutdown terminates connection to alphabill node and closes wallet db
	Shutdown()

	// DeleteDb deletes the wallet database
	DeleteDb() error
}

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func CreateNewWallet(config *config.WalletConfig) (Wallet, error) {
	return wallet.CreateNewWallet(config)
}

// LoadExistingWallet loads an existing wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func LoadExistingWallet(config *config.WalletConfig) (Wallet, error) {
	return wallet.LoadExistingWallet(config)
}
