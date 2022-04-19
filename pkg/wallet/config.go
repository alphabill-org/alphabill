package wallet

import (
	"os"
	"path"
)

// Config configuration options for creating and loading a wallet.
type Config struct {
	// Directory where default boltdb wallet database is created, only used when Db is not set,
	// if empty then 'home/.alphabill' directory is used.
	DbPath string

	// Custom database implementation, if set then DbPath is not used,
	// if not set then boltdb is created at DbPath.
	Db Db

	// WalletPass used to encrypt/decrypt sensitive information. If empty then wallet will not be encrypted.
	WalletPass string

	// Configuration options for connecting to alphabill nodes.
	AlphabillClientConfig AlphabillClientConfig
}

// AlphabillClientConfig configuration options for connecting to alphabill nodes.
type AlphabillClientConfig struct {
	Uri string

	// RequestTimeoutMs timeout for RPC request, if not set then requests never expire
	RequestTimeoutMs uint64
}

// GetWalletDir returns wallet directory,
// if DbPath is set then returns DbPath,
// if DbPath is not set then returns 'home/.alphabill'.
func (c Config) GetWalletDir() (string, error) {
	if c.DbPath != "" {
		return c.DbPath, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return path.Join(homeDir, ".alphabill"), nil
}
