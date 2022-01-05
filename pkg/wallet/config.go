package wallet

import (
	"alphabill-wallet-sdk/internal/abclient"
	"os"
	"path"
)

type Config struct {
	DbPath                string
	Db                    Db
	AlphaBillClientConfig *abclient.AlphaBillClientConfig
}

func (c *Config) GetWalletDir() (string, error) {
	if c.DbPath != "" {
		return c.DbPath, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return path.Join(homeDir, ".alphabill"), nil
}
