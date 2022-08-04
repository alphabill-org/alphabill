package store

import (
	"encoding/json"
	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	bolt "go.etcd.io/bbolt"
)

const BoltBlockStoreFileName = "blocks.db"

var (
	blocksBucket        = []byte("blocksBucket")
	metaBucket          = []byte("metaBucket")
	blockProposalBucket = []byte("blockProposalBucket")
)
var (
	latestBlockNoKey       = []byte("latestBlockNo")
	blockProposalBucketKey = []byte("blockProposal")
)

var errInvalidBlockNo = errors.New("invalid block number")

// BoltBlockStore is a persistent implementation of BlockStore interface.
type BoltBlockStore struct {
	db          *bolt.DB
	latestBlock *block.Block
}

// NewBoltBlockStore creates new on-disk persistent block store using bolt db.
// If the file does not exist then it will be created, however, parent directories must exist beforehand.
func NewBoltBlockStore(dbFile string) (*BoltBlockStore, error) {
	db, err := bolt.Open(dbFile, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}

	bs := &BoltBlockStore{db: db}
	err = bs.createBuckets()
	if err != nil {
		return nil, err
	}
	err = bs.initMetaData()
	if err != nil {
		return nil, err
	}
	height, err := bs.Height()
	if err != nil {
		return nil, err
	}
	bs.latestBlock, err = bs.Get(height)
	if err != nil {
		return nil, err
	}
	logger.Info("Bolt DB initialised")
	return bs, nil
}

func (bs *BoltBlockStore) Add(b *block.Block) error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		err := bs.verifyBlock(tx, b)
		if err != nil {
			return err
		}
		bs.latestBlock = b
		val, err := json.Marshal(b)
		if err != nil {
			return err
		}
		blockNoInBytes := util.Uint64ToBytes(b.BlockNumber)
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

func (bs *BoltBlockStore) Get(blockNumber uint64) (*block.Block, error) {
	var b *block.Block
	err := bs.db.View(func(tx *bolt.Tx) error {
		blockJson := tx.Bucket(blocksBucket).Get(util.Uint64ToBytes(blockNumber))
		if blockJson != nil {
			return json.Unmarshal(blockJson, &b)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (bs *BoltBlockStore) Height() (uint64, error) {
	var height uint64
	err := bs.db.View(func(tx *bolt.Tx) error {
		height = util.BytesToUint64(tx.Bucket(metaBucket).Get(latestBlockNoKey))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return height, nil
}

func (bs *BoltBlockStore) LatestBlock() *block.Block {
	return bs.latestBlock
}

func (bs *BoltBlockStore) AddPendingProposal(proposal *block.PendingBlockProposal) error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(proposal)
		if err != nil {
			return err
		}
		return tx.Bucket(blockProposalBucket).Put(blockProposalBucketKey, val)
	})
}

func (bs *BoltBlockStore) GetPendingProposal() (*block.PendingBlockProposal, error) {
	var bp *block.PendingBlockProposal
	err := bs.db.View(func(tx *bolt.Tx) error {
		blockJson := tx.Bucket(blockProposalBucket).Get(blockProposalBucketKey)
		if blockJson == nil {
			return errors.New(ErrStrPendingBlockProposalNotFound)
		}
		return json.Unmarshal(blockJson, &bp)
	})
	return bp, err
}

func (bs *BoltBlockStore) verifyBlock(tx *bolt.Tx, b *block.Block) error {
	latestBlockNo := bs.getLatestBlockNo(tx)
	if latestBlockNo+1 != b.BlockNumber {
		logger.Warning("Block verification failed: latest block #%v, current block #%v", latestBlockNo, b.BlockNumber)
		return errInvalidBlockNo
	}
	return nil
}

func (bs *BoltBlockStore) getLatestBlockNo(tx *bolt.Tx) uint64 {
	return util.BytesToUint64(tx.Bucket(metaBucket).Get(latestBlockNoKey))
}

func (bs *BoltBlockStore) createBuckets() error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(blockProposalBucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func (bs *BoltBlockStore) initMetaData() error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		val := tx.Bucket(metaBucket).Get(latestBlockNoKey)
		if val == nil {
			err := tx.Bucket(metaBucket).Put(latestBlockNoKey, util.Uint64ToBytes(0))
			if err != nil {
				return err
			}
		}
		return nil
	})
}
