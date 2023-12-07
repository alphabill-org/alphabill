package fees

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/wallet"
)

const (
	FeeManagerDBFileName = "feemanager.db"
)

var (
	bucketAccounts       = []byte("account")
	addFeeContextKey     = []byte("addFeeContext")
	reclaimFeeContextKey = []byte("reclaimFeeContext")
)

type (
	BoltStore struct {
		db *bolt.DB
	}
)

func NewFeeManagerDB(dir string) (*BoltStore, error) {
	dbFile := filepath.Join(dir, FeeManagerDBFileName)
	return NewBoltStore(dbFile)
}

func NewBoltStore(dbFile string) (*BoltStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbFile), 0700); err != nil { // ensure dirs exist
		return nil, err
	}
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second}) // -rw-------
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt DB %s: %w", dbFile, err)
	}
	s := &BoltStore{db: db}
	if err := wallet.CreateBuckets(db.Update, bucketAccounts); err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}
	return s, nil
}

func (s *BoltStore) GetAddFeeContext(accountID []byte) (*AddFeeCreditCtx, error) {
	var feeCtx *AddFeeCreditCtx
	err := s.db.View(func(tx *bolt.Tx) error {
		accountBucket := tx.Bucket(bucketAccounts).Bucket(accountID)
		if accountBucket == nil {
			return nil
		}
		feeCtxBytes := accountBucket.Get(addFeeContextKey)
		if feeCtxBytes == nil {
			return nil
		}
		if err := json.Unmarshal(feeCtxBytes, &feeCtx); err != nil {
			return fmt.Errorf("failed to deserialize add fee credit json: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return feeCtx, nil
}

func (s *BoltStore) SetAddFeeContext(accountID []byte, feeCtx *AddFeeCreditCtx) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		accountBucket, err := tx.Bucket(bucketAccounts).CreateBucketIfNotExists(accountID)
		if err != nil {
			return fmt.Errorf("failed to create account bucket: %x", accountID)
		}
		feeCtxBytes, err := json.Marshal(feeCtx)
		if err != nil {
			return fmt.Errorf("failed to serialize add fee context to json: %w", err)
		}
		return accountBucket.Put(addFeeContextKey, feeCtxBytes)
	})
}

func (s *BoltStore) DeleteAddFeeContext(accountID []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		accountBucket := tx.Bucket(bucketAccounts).Bucket(accountID)
		if accountBucket == nil {
			return nil
		}
		return accountBucket.Delete(addFeeContextKey)
	})
}

func (s *BoltStore) GetReclaimFeeContext(accountID []byte) (*ReclaimFeeCreditCtx, error) {
	var feeCtx *ReclaimFeeCreditCtx
	err := s.db.View(func(tx *bolt.Tx) error {
		accountBucket := tx.Bucket(bucketAccounts).Bucket(accountID)
		if accountBucket == nil {
			return nil
		}
		feeCtxBytes := accountBucket.Get(reclaimFeeContextKey)
		if feeCtxBytes == nil {
			return nil
		}
		if err := json.Unmarshal(feeCtxBytes, &feeCtx); err != nil {
			return fmt.Errorf("failed to deserialize reclaim fee credit json: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return feeCtx, nil
}

func (s *BoltStore) SetReclaimFeeContext(accountID []byte, feeCtx *ReclaimFeeCreditCtx) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		accountBucket, err := tx.Bucket(bucketAccounts).CreateBucketIfNotExists(accountID)
		if err != nil {
			return fmt.Errorf("failed to create account bucket: %x", accountID)
		}
		feeCtxBytes, err := json.Marshal(feeCtx)
		if err != nil {
			return fmt.Errorf("failed to serialize add fee context to json: %w", err)
		}
		return accountBucket.Put(reclaimFeeContextKey, feeCtxBytes)
	})
}

func (s *BoltStore) DeleteReclaimFeeContext(accountID []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		accountBucket := tx.Bucket(bucketAccounts).Bucket(accountID)
		if accountBucket == nil {
			return nil
		}
		return accountBucket.Delete(reclaimFeeContextKey)
	})
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}
