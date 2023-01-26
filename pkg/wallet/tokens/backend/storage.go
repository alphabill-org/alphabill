package twb

import (
	"encoding/json"
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/internal/util"
	wtokens "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
)

var (
	bucketMetadata  = []byte("meta")
	bucketTokenType = []byte("token-type")
	bucketTokenUnit = []byte("token-unit")

	keyBlockHeight = []byte("block-height")
)

var errRecordNotFound = errors.New("not found")

type storage struct {
	db *bolt.DB
}

func (s *storage) SaveTokenType(data *wtokens.TokenUnitType) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize token type data: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketTokenType).Put(data.ID, b)
	})
}

func (s *storage) GetTokenType(id []byte) (*wtokens.TokenUnitType, error) {
	var data []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		if data = tx.Bucket(bucketTokenType).Get(id); data == nil {
			return fmt.Errorf("failed to read token type data %s[%x]: %w", bucketTokenType, id, errRecordNotFound)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	d := &wtokens.TokenUnitType{}
	if err := json.Unmarshal(data, d); err != nil {
		return nil, fmt.Errorf("failed to deserialize token type data (%x): %w", id, err)
	}
	return d, nil
}

func (s *storage) SaveTokenUnit(data *wtokens.TokenUnit) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize token unit data: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketTokenUnit).Put(data.ID, b)
	})
}

func (s *storage) GetBlockNumber() (uint64, error) {
	var blockNumber uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		blockNumberBytes := tx.Bucket(bucketMetadata).Get(keyBlockHeight)
		if blockNumberBytes == nil {
			return fmt.Errorf("block number not stored (%s->%s)", bucketMetadata, keyBlockHeight)
		}
		blockNumber = util.BytesToUint64(blockNumberBytes)
		return nil
	})
	return blockNumber, err
}

func (s *storage) SetBlockNumber(blockNumber uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(keyBlockHeight, util.Uint64ToBytes(blockNumber))
	})
}

func (s *storage) Close() error { return s.db.Close() }

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
		val := tx.Bucket(bucketMetadata).Get(keyBlockHeight)
		if val == nil {
			return tx.Bucket(bucketMetadata).Put(keyBlockHeight, util.Uint64ToBytes(0))
		}
		return nil
	})
}

func newBoltStore(dbFile string) (*storage, error) {
	db, err := bolt.Open(dbFile, 0600, nil) // -rw-------
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt DB: %w", err)
	}
	s := &storage{db: db}

	if err := s.createBuckets(bucketMetadata, bucketTokenType, bucketTokenUnit); err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}

	if err := s.initMetaData(); err != nil {
		return nil, fmt.Errorf("failed to init db metadata: %w", err)
	}

	return s, nil
}
