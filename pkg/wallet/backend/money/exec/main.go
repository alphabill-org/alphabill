package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
)

//http://localhost:8080/api/v1/list-bills?pubkey=0x000000000000000000000000000000000000000000000000000000000000000000
func main() {
	dbFile := filepath.Join(os.TempDir(), "backend.db")
	fmt.Printf("Backend db file: %s", dbFile)
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			panic(err)
		}
	}(dbFile)
	err := money.CreateAndRun(context.Background(), &money.Config{
		ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
		//AlphabillUrl:            "", // empty string will not start the sync
		AlphabillUrl:       "localhost:26766",
		ServerAddr:         "localhost:8080",
		DbFile:             dbFile,
		ListBillsPageLimit: 100,
	})
	if err != nil {
		return
	}
}
