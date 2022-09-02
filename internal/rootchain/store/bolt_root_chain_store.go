package store

import (
	"encoding/json"
	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
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

type BoltRootChainStore struct {
	db *bolt.DB
}

func NewBoltRootChainStore(dbFile string) (*BoltRootChainStore, error) {
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		return nil, err
	}
	s := &BoltRootChainStore{db: db}
	err = s.createBuckets()
	if err != nil {
		return nil, err
	}
	err = s.initRound()
	if err != nil {
		return nil, err
	}
	logger.Info("Bolt DB initialised")

	return s, err
}

func (s *BoltRootChainStore) createBuckets() error {
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

func (s *BoltRootChainStore) initRound() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		nr := tx.Bucket(roundBucket).Get(latestRoundNumberKey)
		if nr == nil {
			err := tx.Bucket(roundBucket).Put(latestRoundNumberKey, util.Uint64ToBytes(1))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BoltRootChainStore) AddUC(id p.SystemIdentifier, certificate *certificates.UnicityCertificate) {
	err := s.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(certificate)
		if err != nil {
			return err
		}
		err = tx.Bucket(ucBucket).Put([]byte(id), val)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func (s *BoltRootChainStore) GetUC(id p.SystemIdentifier) *certificates.UnicityCertificate {
	var uc *certificates.UnicityCertificate
	err := s.db.View(func(tx *bolt.Tx) error {
		if ucJson := tx.Bucket(ucBucket).Get([]byte(id)); ucJson != nil {
			return json.Unmarshal(ucJson, &uc)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return uc
}

func (s *BoltRootChainStore) UCCount() int {
	var keysCount int
	err := s.db.View(func(tx *bolt.Tx) error {
		keysCount = tx.Bucket(ucBucket).Stats().KeyN
		return nil
	})
	if err != nil {
		panic(err)
	}
	return keysCount
}

func (s *BoltRootChainStore) AddIR(p.SystemIdentifier, *certificates.InputRecord) {
	// TODO
	panic("TODO")
}

func (s *BoltRootChainStore) GetIR(p.SystemIdentifier) *certificates.InputRecord {
	panic("TODO")
}

func (s *BoltRootChainStore) GetAllIRs() map[p.SystemIdentifier]*certificates.InputRecord {
	panic("TODO")
}

func (s *BoltRootChainStore) GetRoundNumber() uint64 {
	var roundNr uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		roundNr = util.BytesToUint64(tx.Bucket(roundBucket).Get(latestRoundNumberKey))
		return nil
	})
	if err != nil {
		panic(err)
	}
	return roundNr
}

func (s *BoltRootChainStore) GetPreviousRoundRootHash() []byte {
	var hash []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		hash = tx.Bucket(prevRoundHashBucket).Get(prevRoundHashKey)
		return nil
	})
	if err != nil {
		panic(err)
	}
	return hash
}

func (s *BoltRootChainStore) PrepareNextRound(prevStateHash []byte) uint64 {
	var roundNr uint64
	err := s.db.Update(func(tx *bolt.Tx) error {
		roundNr = util.BytesToUint64(tx.Bucket(roundBucket).Get(latestRoundNumberKey))
		roundNr++
		if err := tx.Bucket(roundBucket).Put(latestRoundNumberKey, util.Uint64ToBytes(roundNr)); err != nil {
			return err
		}
		return tx.Bucket(prevRoundHashBucket).Put(prevRoundHashKey, prevStateHash)
	})
	if err != nil {
		panic(err)
	}
	return roundNr
}
