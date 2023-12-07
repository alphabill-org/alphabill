package unitlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	sdk "github.com/alphabill-org/alphabill/wallet"
)

var (
	bucketAccounts = []byte("account")
)

type BoltStore struct {
	db *bolt.DB
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
	if err := sdk.CreateBuckets(db.Update, bucketAccounts); err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}
	return s, nil
}

func (s *BoltStore) GetUnit(accountID, unitID []byte) (*LockedUnit, error) {
	var unit *LockedUnit
	err := s.db.View(func(tx *bolt.Tx) error {
		unitsBucket, err := s.getUnitsBucket(tx, accountID)
		if err != nil {
			return err
		}
		if unitsBucket == nil {
			return nil
		}
		unitBytes := unitsBucket.Get(unitID)
		if unitBytes == nil {
			return nil
		}
		if err := json.Unmarshal(unitBytes, &unit); err != nil {
			return fmt.Errorf("failed to deserialize unit data: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return unit, nil
}

func (s *BoltStore) GetUnits(accountID []byte) ([]*LockedUnit, error) {
	var units []*LockedUnit
	err := s.db.View(func(tx *bolt.Tx) error {
		unitsBucket, err := s.getUnitsBucket(tx, accountID)
		if err != nil {
			return err
		}
		if unitsBucket == nil {
			return nil
		}
		return unitsBucket.ForEach(func(k, v []byte) error {
			var unit *LockedUnit
			if err := json.Unmarshal(v, &unit); err != nil {
				return fmt.Errorf("failed to deserialize unit data: %w", err)
			}
			units = append(units, unit)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return units, nil
}

func (s *BoltStore) PutUnit(unit *LockedUnit) error {
	if err := unit.isValid(); err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		accountsBucket := tx.Bucket(bucketAccounts)
		if accountsBucket == nil {
			return fmt.Errorf("accounts bucket does not exist")
		}
		unitsBucket, err := accountsBucket.CreateBucketIfNotExists(unit.AccountID)
		if err != nil {
			return fmt.Errorf("failed to load units bucket for account %x: %w", unit.AccountID, err)
		}
		unitBytes, err := json.Marshal(unit)
		if err != nil {
			return fmt.Errorf("failed to serialize unit to json: %w", err)
		}
		return unitsBucket.Put(unit.UnitID, unitBytes)
	})
}

func (s *BoltStore) DeleteUnit(accountID, unitID []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		accountsBucket := tx.Bucket(bucketAccounts)
		if accountsBucket == nil {
			return errors.New("accounts bucket does not exist")
		}
		unitsBucket := accountsBucket.Bucket(accountID)
		if unitsBucket == nil {
			return fmt.Errorf("units bucket for account %x does not exist", accountID)
		}
		return unitsBucket.Delete(unitID)
	})
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}

func (s *BoltStore) getUnitsBucket(tx *bolt.Tx, accountID []byte) (*bolt.Bucket, error) {
	accountsBucket := tx.Bucket(bucketAccounts)
	if accountsBucket == nil {
		return nil, errors.New("accounts bucket does not exist")
	}
	return accountsBucket.Bucket(accountID), nil
}
