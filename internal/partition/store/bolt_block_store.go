package store

import (
	"encoding/json"
	"log"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/partition/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
	bolt "go.etcd.io/bbolt"
)

const BoltBlockStoreFileName = "blocks.db"

var (
	blocksBucket        = []byte("blocksBucket")
	metaBucket          = []byte("metaBucket")
	blockProposalBucket = []byte("blockProposalBucket")
	latestUCBucket      = []byte("latestUCBucket")
)
var (
	latestBlockNoKey       = []byte("latestBlockNo") // latest persisted non-empty block number
	latestRoundNoKey       = []byte("latestRoundNo") // latest certified round number (can be equal or greater than the latest block number)
	blockProposalBucketKey = []byte("blockProposal")
	latestUCBucketKey      = []byte("latestUC")
)

var errInvalidBlockNo = errors.New("invalid block number")

// BoltBlockStore is a persistent implementation of BlockStore interface.
type BoltBlockStore struct {
	db *bolt.DB
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
	logger.Info("Bolt DB initialised")
	return bs, nil
}

func (bs *BoltBlockStore) Add(b *block.Block) error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		if err := bs.verifyBlock(tx, b); err != nil {
			return err
		}
		roundNoInBytes := util.Uint64ToBytes(b.UnicityCertificate.InputRecord.RoundNumber)
		if err := tx.Bucket(metaBucket).Put(latestRoundNoKey, roundNoInBytes); err != nil {
			return err
		}
		uc, err := json.Marshal(b.UnicityCertificate)
		if err != nil {
			return err
		}
		if err := tx.Bucket(latestUCBucket).Put(latestUCBucketKey, uc); err != nil {
			return err
		}

		if len(b.Transactions) == 0 {
			return nil
		}
		val, err := json.Marshal(b)
		if err != nil {
			return err
		}
		blockNoInBytes := util.Uint64ToBytes(b.UnicityCertificate.InputRecord.RoundNumber)
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

func (bs *BoltBlockStore) AddGenesis(b *block.Block) error {
	return bs.db.Update(func(tx *bolt.Tx) error {
		if err := bs.verifyBlock(tx, b); err != nil {
			return err
		}
		if b.UnicityCertificate.InputRecord.RoundNumber > genesis.GenesisRoundNumber {
			return errors.Errorf("genesis block number must be %v", genesis.GenesisRoundNumber)
		}
		roundNoInBytes := util.Uint64ToBytes(b.UnicityCertificate.InputRecord.RoundNumber)
		if err := tx.Bucket(metaBucket).Put(latestRoundNoKey, roundNoInBytes); err != nil {
			return err
		}
		uc, err := json.Marshal(b.UnicityCertificate)
		if err != nil {
			return err
		}
		if err := tx.Bucket(latestUCBucket).Put(latestUCBucketKey, uc); err != nil {
			return err
		}
		val, err := json.Marshal(b)
		if err != nil {
			return err
		}
		blockNoInBytes := util.Uint64ToBytes(b.UnicityCertificate.InputRecord.RoundNumber)
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

func (bs *BoltBlockStore) LatestRoundNumber() uint64 {
	var number uint64
	err := bs.db.View(func(tx *bolt.Tx) error {
		number = util.BytesToUint64(tx.Bucket(metaBucket).Get(latestRoundNoKey))
		return nil
	})
	if err != nil {
		panic(err)
	}
	return number
}
func (bs *BoltBlockStore) LatestUC() *certificates.UnicityCertificate {
	var uc *certificates.UnicityCertificate
	err := bs.db.View(func(tx *bolt.Tx) error {
		ucBytes := tx.Bucket(latestUCBucket).Get(latestUCBucketKey)
		if ucBytes != nil {
			return json.Unmarshal(ucBytes, &uc)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return uc
}

func (bs *BoltBlockStore) BlockNumber() (uint64, error) {
	var number uint64
	err := bs.db.View(func(tx *bolt.Tx) error {
		number = util.BytesToUint64(tx.Bucket(metaBucket).Get(latestBlockNoKey))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return number, nil
}

func (bs *BoltBlockStore) LatestBlock() *block.Block {
	var res *block.Block
	err := bs.db.View(func(tx *bolt.Tx) error {
		latestBlockNumber := tx.Bucket(metaBucket).Get(latestBlockNoKey)
		blockJson := tx.Bucket(blocksBucket).Get(latestBlockNumber)
		if blockJson == nil {
			return nil
		}
		return json.Unmarshal(blockJson, &res)
	})
	if err != nil {
		log.Panicf("error fetching latest block %v", err)
	}
	return res
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
	latestRoundNo := bs.getLatestRoundNo(tx)
	if latestRoundNo+1 != b.UnicityCertificate.InputRecord.RoundNumber {
		logger.Warning("Block verification failed: latest certified block #%v, current block's round #%v", latestRoundNo, b.UnicityCertificate.InputRecord.RoundNumber)
		return errInvalidBlockNo
	}
	return nil
}

func (bs *BoltBlockStore) getLatestRoundNo(tx *bolt.Tx) uint64 {
	return util.BytesToUint64(tx.Bucket(metaBucket).Get(latestBlockNoKey))
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
		_, err = tx.CreateBucketIfNotExists(latestUCBucket)
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
