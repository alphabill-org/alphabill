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

	// Configuration options for connecting to alphabill nodes.
	AlphaBillClientConfig AlphaBillClientConfig
}

// AlphaBillClientConfig configuration options for connecting to alphabill nodes.
type AlphaBillClientConfig struct {
	Uri string
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
