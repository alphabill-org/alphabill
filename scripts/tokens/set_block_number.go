package main

import (
	"flag"
	"log"

	"github.com/alphabill-org/alphabill/internal/util"
	bolt "go.etcd.io/bbolt"
)

/*
Example usage
go run scripts/tokens/set_block_number.go --file /path/to/tokens.db --block 100
*/
func main() {
	// parse command line parameters
	pathToBoltDb := flag.String("file", "", "path to .db file")
	block := flag.Uint64("block", 0, "latest block number to set")
	flag.Parse()

	// verify command line parameters
	if *pathToBoltDb == "" {
		log.Fatal("pathToBoltDb is required")
	}
	if *block == 0 {
		log.Fatal("block is required")
	}

	db, err := openTokensDb(*pathToBoltDb)
	if err != nil {
		panic(err)
	}
	defer func(db *bolt.DB) {
		err := db.Close()
		if err != nil {
			panic(err)
		}
	}(db.db)

	err = db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(blockHeightKeyName, util.Uint64ToBytes(*block))
	})
	if err != nil {
		panic(err)
	}
}

var (
	metaBucket         = []byte("meta")
	blockHeightKeyName = []byte("blockHeightKey")
)

type tokensDb struct {
	db         *bolt.DB
	dbFilePath string
}

type tokensDbTx struct {
	db *tokensDb
	tx *bolt.Tx
}

func openTokensDb(dbFilePath string) (*tokensDb, error) {
	db, err := bolt.Open(dbFilePath, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}
	w := &tokensDb{db, dbFilePath}
	return w, nil
}
