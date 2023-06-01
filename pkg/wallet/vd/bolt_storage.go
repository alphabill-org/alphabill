package wallet

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	bucketMetadata = []byte("meta")
	bucketFeeCredits = []byte("fee-credits") // UnitID -> json(FeeCreditBill)

	keyBlockNumber = []byte("block-number")
)

type (
	storage struct {
		db *bolt.DB
	}

	FeeCreditBill struct {
		Id            []byte // unitID
		Value         uint64 // fee credit balance
		TxHash        []byte // hash of the transaction that last updated fee credit balance
		FCBlockNumber uint64 // number of the block that last updated fee credit balance
	}
)

func (s *storage) Close() error { return s.db.Close() }

func (s *storage) GetBlockNumber() (uint64, error) {
	var blockNumber uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		blockNumberBytes := tx.Bucket(bucketMetadata).Get(keyBlockNumber)
		if blockNumberBytes == nil {
			return fmt.Errorf("block number not stored (%s->%s)", bucketMetadata, keyBlockNumber)
		}
		blockNumber = util.BytesToUint64(blockNumberBytes)
		return nil
	})
	return blockNumber, err
}

func (s *storage) SetBlockNumber(blockNumber uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(keyBlockNumber, util.Uint64ToBytes(blockNumber))
	})
}

func (s *storage) GetFeeCreditBill(unitID wallet.UnitID) (*FeeCreditBill, error) {
	var fcb *FeeCreditBill
	err := s.db.View(func(tx *bolt.Tx) error {
		fcbBytes := tx.Bucket(bucketFeeCredits).Get(unitID)
		if fcbBytes == nil {
			return nil
		}
		return json.Unmarshal(fcbBytes, &fcb)
	})

	if err != nil {
		return nil, err
	}
	return fcb, err
}

func (s *storage) SetFeeCreditBill(fcb *FeeCreditBill) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		fcbBytes, err := json.Marshal(fcb)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketFeeCredits).Put(fcb.Id, fcbBytes)
	})
}

func (s *storage) createBuckets(buckets ...[]byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("failed to create bucket %q: %w", b, err)
			}
		}
		return nil
	})
}

func (s *storage) initMetaData() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		val := tx.Bucket(bucketMetadata).Get(keyBlockNumber)
		if val == nil {
			return tx.Bucket(bucketMetadata).Put(keyBlockNumber, util.Uint64ToBytes(0))
		}
		return nil
	})
}

func (f *FeeCreditBill) GetValue() uint64 {
	if f != nil {
		return f.Value
	}
	return 0
}

func newBoltStore(dbFile string) (*storage, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second}) // -rw-------
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt DB: %w", err)
	}
	s := &storage{db: db}

	if err := s.createBuckets(bucketMetadata, bucketFeeCredits); err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}

	if err := s.initMetaData(); err != nil {
		return nil, fmt.Errorf("failed to init db metadata: %w", err)
	}

	return s, nil
}
