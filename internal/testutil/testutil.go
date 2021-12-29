package testutil

import (
	"os"
)

func DeleteWalletDb() error {
	dir, _ := getWalletDir()
	dbFilePath := dir + "wallet.db"
	return os.Remove(dbFilePath)
}

func getWalletDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	walletDir := homeDir + string(os.PathSeparator) + ".alphabill" + string(os.PathSeparator)
	return walletDir, nil
}
