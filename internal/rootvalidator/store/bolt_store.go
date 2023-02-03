package store

import (
	"encoding/json"
	"errors"

	bolt "go.etcd.io/bbolt"
)

const BoltRootChainStoreFileName = "rootchain.db"
const defaultBucket = "default"

var (
	ErrValueEmpty = errors.New("empty")
	ErrInvalidKey = errors.New("invalid key")
	ErrValueIsNil = errors.New("value is nil")
)

type BoltStore struct {
	db     *bolt.DB
	bucket string
}

func checkKeyAndValue(key string, val any) error {
	if len(key) == 0 {
		return ErrInvalidKey
	}
	if val == nil {
		return ErrValueIsNil
	}
	return nil
}

func NewBoltStore(dbFile string) (*BoltStore, error) {
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		return nil, err
	}
	s := &BoltStore{db: db, bucket: defaultBucket}
	err = s.createBuckets()
	if err != nil {
		return nil, err
	}
	return s, err
}

func (s *BoltStore) createBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(s.bucket))
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *BoltStore) Read(key string, v any) error {
	if err := checkKeyAndValue(key, v); err != nil {
		return err
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte(s.bucket)).Get([]byte(key))
		// nil is returned if no value is stored under key
		if data != nil {
			return json.Unmarshal(data, v)
		}
		return ErrValueEmpty
	})
	return err
}

func (s *BoltStore) Write(key string, v any) error {
	if err := checkKeyAndValue(key, v); err != nil {
		return err
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(s.bucket)).Put([]byte(key), b)
	})
	return err
}
