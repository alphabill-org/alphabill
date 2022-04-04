package partition

import (
	"encoding/binary"
	"encoding/json"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	bolt "go.etcd.io/bbolt"
)

const blocksDbFileName = "blocks.db"

var (
	blocksBucket = []byte("blocksBucket")
	metaBucket   = []byte("metaBucket")
)

var latestBlockNoKey = []byte("latestBlockNo")

var errInvalidBlockNo = errors.New("invalid block number")

// PersistentBlockStore is a persistent implementation of BlockStore interface.
type PersistentBlockStore struct {
	db *bolt.DB
}

// NewPersistentBlockStore creates new on-disk persistent block store.
// If the file does not exist then it will be created, however, parent directories must exist beforehand.
func NewPersistentBlockStore(dbFile string) (*PersistentBlockStore, error) {
	db, err := bolt.Open(dbFile, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}

	bs := &PersistentBlockStore{db}
	err = bs.createBuckets()
	if err != nil {
		return nil, err
	}
	err = bs.initMetaData()
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func (bs *PersistentBlockStore) Add(b *Block) error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		err := bs.verifyBlock(tx, b)
		if err != nil {
			return err
		}
		val, err := json.Marshal(b)
		if err != nil {
			return err
		}
		blockNoInBytes := serializeUint64(b.TxSystemBlockNumber)
		err = tx.Bucket(blocksBucket).Put(blockNoInBytes, val)
		if err != nil {
			return err
		}
		err = tx.Bucket(metaBucket).Put(latestBlockNoKey, blockNoInBytes)
		if err != nil {
			return err
		}
		return nil
	})
}

func (bs *PersistentBlockStore) Get(blockNumber uint64) (*Block, error) {
	var block *Block
	err := bs.db.View(func(tx *bolt.Tx) error {
		blockJson := tx.Bucket(blocksBucket).Get(serializeUint64(blockNumber))
		if blockJson != nil {
			return json.Unmarshal(blockJson, &block)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (bs *PersistentBlockStore) Height() (uint64, error) {
	var height uint64
	err := bs.db.View(func(tx *bolt.Tx) error {
		height = deserializeUint64(tx.Bucket(metaBucket).Get(latestBlockNoKey))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return height, nil
}

func (bs *PersistentBlockStore) LatestBlock() (*Block, error) {
	height, err := bs.Height()
	if err != nil {
		return nil, err
	}
	return bs.Get(height)
}

func (bs *PersistentBlockStore) verifyBlock(tx *bolt.Tx, b *Block) error {
	latestBlockNo := bs.getLatestBlockNo(tx)
	if latestBlockNo+1 != b.TxSystemBlockNumber {
		return errInvalidBlockNo
	}
	return nil
}

func (bs *PersistentBlockStore) getLatestBlockNo(tx *bolt.Tx) uint64 {
	return deserializeUint64(tx.Bucket(metaBucket).Get(latestBlockNoKey))
}

func (bs *PersistentBlockStore) createBuckets() error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func (bs *PersistentBlockStore) initMetaData() error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		val := tx.Bucket(metaBucket).Get(latestBlockNoKey)
		if val == nil {
			err := tx.Bucket(metaBucket).Put(latestBlockNoKey, serializeUint64(0))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func serializeUint64(key uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, key)
	return b
}

func deserializeUint64(key []byte) uint64 {
	return binary.BigEndian.Uint64(key)
}
