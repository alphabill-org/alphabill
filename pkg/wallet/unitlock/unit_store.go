package unitlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
)

var (
	bucketUnits = []byte("units")
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
	if err := sdk.CreateBuckets(db.Update, bucketUnits); err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}
	return s, nil
}

func (s *BoltStore) GetUnit(unitID []byte) (*LockedUnit, error) {
	var unit *LockedUnit
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketUnits)
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketUnits)
		}
		unitData := bucket.Get(unitID)
		if unitData == nil {
			return nil
		}
		if err := json.Unmarshal(unitData, &unit); err != nil {
			return fmt.Errorf("failed to deserialize unit data: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return unit, nil
}

func (s *BoltStore) GetUnits() ([]*LockedUnit, error) {
	var units []*LockedUnit
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketUnits)
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketUnits)
		}
		return bucket.ForEach(func(k, v []byte) error {
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
	if unit == nil {
		return errors.New("unit is nil")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketUnits)
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketUnits)
		}
		unitBytes, err := json.Marshal(unit)
		if err != nil {
			return err
		}
		return bucket.Put(unit.UnitID, unitBytes)
	})
}

func (s *BoltStore) DeleteUnit(unitID []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketUnits)
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketUnits)
		}
		return bucket.Delete(unitID)
	})
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}
