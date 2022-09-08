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
	irBucket            = []byte("irBucket")

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
		_, err = tx.CreateBucketIfNotExists(irBucket)
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

func (s *BoltRootChainStore) AddIR(id p.SystemIdentifier, ir *certificates.InputRecord) {
	err := s.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(ir)
		if err != nil {
			return err
		}
		err = tx.Bucket(irBucket).Put([]byte(id), val)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func (s *BoltRootChainStore) GetIR(id p.SystemIdentifier) *certificates.InputRecord {
	var ir *certificates.InputRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		if ucJson := tx.Bucket(irBucket).Get([]byte(id)); ucJson != nil {
			return json.Unmarshal(ucJson, &ir)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return ir
}

func (s *BoltRootChainStore) GetAllIRs() map[p.SystemIdentifier]*certificates.InputRecord {
	target := make(map[p.SystemIdentifier]*certificates.InputRecord)
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(irBucket).ForEach(func(k, v []byte) error {
			var ir *certificates.InputRecord
			if err := json.Unmarshal(v, &ir); err != nil {
				return err
			}
			target[p.SystemIdentifier(k)] = ir
			return nil
		})
	})
	if err != nil {
		panic(err)
	}
	return target
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

func (s *BoltRootChainStore) PrepareNextRound(prevStateHash []byte, ucs []*certificates.UnicityCertificate) uint64 {
	var roundNr uint64
	err := s.db.Update(func(tx *bolt.Tx) error {
		roundNr = util.BytesToUint64(tx.Bucket(roundBucket).Get(latestRoundNumberKey))
		roundNr++
		if err := tx.Bucket(roundBucket).Put(latestRoundNumberKey, util.Uint64ToBytes(roundNr)); err != nil {
			return err
		}
		certsBucket := tx.Bucket(ucBucket)
		for _, cert := range ucs {
			val, err := json.Marshal(cert)
			if err != nil {
				return err
			}
			err = certsBucket.Put(cert.UnicityTreeCertificate.SystemIdentifier, val)
			if err != nil {
				return err
			}
		}
		// clean IRs
		_ = tx.DeleteBucket(irBucket)
		_, _ = tx.CreateBucketIfNotExists(irBucket)

		return tx.Bucket(prevRoundHashBucket).Put(prevRoundHashKey, prevStateHash)
	})
	if err != nil {
		panic(err)
	}
	return roundNr
}
