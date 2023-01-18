package store

import (
	"encoding/json"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/util"
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

func (s *BoltStore) Read() (*RootState, error) {
	state := NewRootState()
	err := s.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(roundBucket).Get(latestRoundNumberKey)
		// nil is returned if no value is stored under key
		if val != nil {
			state.LatestRound = util.BytesToUint64(val)
		}
		state.LatestRootHash = tx.Bucket(prevRoundHashBucket).Get(prevRoundHashKey)
		err := tx.Bucket(ucBucket).ForEach(func(k, v []byte) error {
			var uc *certificates.UnicityCertificate
			if err := json.Unmarshal(v, &uc); err != nil {
				return err
			}
			state.Certificates[protocol.SystemIdentifier(k)] = uc
			return nil
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (s *BoltStore) Write(newState *RootState) error {
	if newState == nil {
		return fmt.Errorf("state is nil")
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
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
