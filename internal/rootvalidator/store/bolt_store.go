package store

import (
	"encoding/json"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

const BoltRootChainStoreFileName = "rootchain.db"

var (
	// buckets
	prevRoundHashBucket = []byte("prevRoundHashBucket")
	roundBucket         = []byte("roundBucket")
	ucBucket            = []byte("ucBucket")

	// keys
	prevRoundHashKey     = []byte("prevRoundHashKey")
	latestRoundNumberKey = []byte("latestRoundNumberKey")
)

type BoltStore struct {
	db *bolt.DB
}

func NewBoltStore(dbFile string) (*BoltStore, error) {
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		return nil, err
	}
	s := &BoltStore{db: db}
	err = s.createBuckets()
	if err != nil {
		return nil, err
	}
	return s, err
}

func (s *BoltStore) createBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(roundBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(prevRoundHashBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(ucBucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *BoltStore) GetUC(id protocol.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	var uc *certificates.UnicityCertificate
	err := s.db.View(func(tx *bolt.Tx) error {
		if ucJson := tx.Bucket(ucBucket).Get([]byte(id)); ucJson != nil {
			return json.Unmarshal(ucJson, &uc)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return uc, nil
}

func (s *BoltStore) ReadAllUC() (map[protocol.SystemIdentifier]*certificates.UnicityCertificate, error) {
	target := make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(ucBucket).ForEach(func(k, v []byte) error {
			var uc *certificates.UnicityCertificate
			if err := json.Unmarshal(v, &uc); err != nil {
				return err
			}
			target[protocol.SystemIdentifier(k)] = uc
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return target, nil
}

func (s *BoltStore) CountUC() (int, error) {
	var keysCount int
	err := s.db.View(func(tx *bolt.Tx) error {
		keysCount = tx.Bucket(ucBucket).Stats().KeyN
		return nil
	})
	if err != nil {
		return -1, err
	}
	return keysCount, nil
}

func (s *BoltStore) ReadLatestRoundNumber() (uint64, error) {
	var roundNr uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(roundBucket).Get(latestRoundNumberKey)
		if val == nil {
			return nil
		}
		roundNr = util.BytesToUint64(val)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return roundNr, nil
}

func (s *BoltStore) ReadLatestRoundRootHash() ([]byte, error) {
	var hash []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		hash = tx.Bucket(prevRoundHashBucket).Get(prevRoundHashKey)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hash, nil
}

func (s *BoltStore) WriteState(newState RootState) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		var latestRoundNr uint64
		val := tx.Bucket(roundBucket).Get(latestRoundNumberKey)
		if val != nil {
			latestRoundNr = util.BytesToUint64(tx.Bucket(roundBucket).Get(latestRoundNumberKey))
		}
		if latestRoundNr+1 != newState.LatestRound {
			return errors.Errorf("Inconsistent round number, current=%v, new=%v", latestRoundNr, newState.LatestRound)
		}
		if err := tx.Bucket(roundBucket).Put(latestRoundNumberKey, util.Uint64ToBytes(newState.LatestRound)); err != nil {
			return err
		}
		certsBucket := tx.Bucket(ucBucket)
		for _, cert := range newState.Certificates {
			val, err := json.Marshal(cert)
			if err != nil {
				return err
			}
			err = certsBucket.Put(cert.UnicityTreeCertificate.SystemIdentifier, val)
			if err != nil {
				return err
			}
		}

		return tx.Bucket(prevRoundHashBucket).Put(prevRoundHashKey, newState.LatestRootHash)
	})
	if err != nil {
		return err
	}
	return nil
}
