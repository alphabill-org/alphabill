package testutil

import (
	"os"
)

func DeleteWalletDb(walletDir string) error {
	dbFilePath := walletDir + "wallet.db"
	return os.Remove(dbFilePath)
}
