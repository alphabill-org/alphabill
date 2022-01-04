package config

import (
	"os"
	"path"
)

type WalletConfig struct {
	DbPath string
	//Db                    wallet.Db
	AlphaBillClientConfig *AlphaBillClientConfig
}

func (c *WalletConfig) GetWalletDir() (string, error) {
	if c.DbPath != "" {
		return c.DbPath, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return path.Join(homeDir, ".alphabill"), nil
}

type AlphaBillClientConfig struct {
	Uri string
}
