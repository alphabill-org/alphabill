package backend

import (
	"encoding/json"
	"errors"

	"github.com/alphabill-org/alphabill/internal/util"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	bolt "go.etcd.io/bbolt"
)

const BoltBillStoreFileName = "bills.db"

var (
	pubkeyIndexBucket = []byte("pubkeyIndexBucket") // pubkey => bucket[bill_id]=bill
	keysBucket        = []byte("keysBucket")        // pubkey => hashed pubkey
	metaBucket        = []byte("metaBucket")        // block_number_key => block_number_val; pubkey => pubkey_block_order_number

	blockNumberKey = []byte("blockNumberKey")
)

var (
	ErrKeyAlreadyExists = errors.New("key already exists")
	ErrPubKeyNotIndexed = errors.New("pubkey not indexed")
	ErrBillNotFound     = errors.New("bill does not exist")
)

type BoltBillStore struct {
	db *bolt.DB
}

// NewBoltBillStore creates new on-disk persistent storage for bills and proofs using bolt db.
// If the file does not exist then it will be created, however, parent directories must exist beforehand.
func NewBoltBillStore(dbFile string) (*BoltBillStore, error) {
	db, err := bolt.Open(dbFile, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}
	s := &BoltBillStore{db: db}
	err = s.createBuckets()
	if err != nil {
		return nil, err
	}
	err = s.initMetaData()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *BoltBillStore) GetBlockNumber() (uint64, error) {
	blockNumber := uint64(0)
	err := s.db.View(func(tx *bolt.Tx) error {
		blockNumberBytes := tx.Bucket(metaBucket).Get(blockNumberKey)
		blockNumber = util.BytesToUint64(blockNumberBytes)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

func (s *BoltBillStore) SetBlockNumber(blockNumber uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		blockNumberBytes := util.Uint64ToBytes(blockNumber)
		err := tx.Bucket(metaBucket).Put(blockNumberKey, blockNumberBytes)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *BoltBillStore) GetBills(pubkey []byte) ([]*Bill, error) {
	var bills []*Bill
	err := s.db.View(func(tx *bolt.Tx) error {
		exists := tx.Bucket(keysBucket).Get(pubkey)
		if exists == nil {
			return ErrPubKeyNotIndexed
		}
		billsBucket := tx.Bucket(pubkeyIndexBucket).Bucket(pubkey)
		if billsBucket == nil {
			return nil // key is indexed but nothing is added yet
		}
		return billsBucket.ForEach(func(billId, billBytes []byte) error {
			var b *Bill
			err := json.Unmarshal(billBytes, &b)
			if err != nil {
				return err
			}
			bills = append(bills, b)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return bills, nil
}

func (s *BoltBillStore) RemoveBill(pubkey []byte, unitID []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		billsBucket := tx.Bucket(pubkeyIndexBucket).Bucket(pubkey)
		if billsBucket == nil {
			return nil
		}
		return billsBucket.Delete(unitID)
	})
}

func (s *BoltBillStore) ContainsBill(pubkey []byte, unitID []byte) (bool, error) {
	res := false
	err := s.db.View(func(tx *bolt.Tx) error {
		billsBucket := tx.Bucket(pubkeyIndexBucket).Bucket(pubkey)
		if billsBucket == nil {
			return nil
		}
		exists := billsBucket.Get(unitID)
		if exists != nil {
			res = true
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return res, nil
}

func (s *BoltBillStore) GetBill(pubkey []byte, unitID []byte) (*Bill, error) {
	var bill *Bill
	err := s.db.View(func(tx *bolt.Tx) error {
		exists := tx.Bucket(keysBucket).Get(pubkey)
		if exists == nil {
			return ErrPubKeyNotIndexed
		}
		billsBucket := tx.Bucket(pubkeyIndexBucket).Bucket(pubkey)
		if billsBucket == nil {
			return ErrBillNotFound
		}
		billBytes := billsBucket.Get(unitID)
		if len(billBytes) == 0 {
			return ErrBillNotFound
		}
		err := json.Unmarshal(billBytes, &bill)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bill, nil
}

func (s *BoltBillStore) SetBills(pubkey []byte, bills ...*Bill) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		exists := tx.Bucket(keysBucket).Get(pubkey)
		if exists == nil {
			return ErrPubKeyNotIndexed
		}
		billsBucket, err := tx.Bucket(pubkeyIndexBucket).CreateBucketIfNotExists(pubkey)
		if err != nil {
			return err
		}
		for _, bill := range bills {
			billOrderNumber := s.getMaxBillOrderNumber(tx, pubkey)
			bill.OrderNumber = billOrderNumber + 1
			billBytes, err := json.Marshal(bill)
			if err != nil {
				return err
			}
			err = billsBucket.Put(bill.Id, billBytes)
			if err != nil {
				return err
			}
			err = tx.Bucket(metaBucket).Put(pubkey, util.Uint64ToBytes(bill.OrderNumber))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BoltBillStore) GetKeys() ([]*Pubkey, error) {
	var keys []*Pubkey
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(keysBucket).ForEach(func(_, pubkeyBytes []byte) error {
			var key *Pubkey
			err := json.Unmarshal(pubkeyBytes, &key)
			if err != nil {
				return err
			}
			keys = append(keys, key)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *BoltBillStore) GetKey(pubkey []byte) (*Pubkey, error) {
	var key *Pubkey
	err := s.db.View(func(tx *bolt.Tx) error {
		keyBytes := tx.Bucket(keysBucket).Get(pubkey)
		if keyBytes != nil {
			err := json.Unmarshal(keyBytes, &key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *BoltBillStore) AddKey(k *Pubkey) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		keysBkt := tx.Bucket(keysBucket)
		exists := keysBkt.Get(k.Pubkey)
		if exists == nil {
			keyBytes, err := json.Marshal(k)
			if err != nil {
				return err
			}
			wlog.Info("adding new key to indexer: ", hexutil.Encode(k.Pubkey))
			return keysBkt.Put(k.Pubkey, keyBytes)
		}
		return ErrKeyAlreadyExists
	})
}

func (s *BoltBillStore) createBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(pubkeyIndexBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(keysBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *BoltBillStore) initMetaData() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		val := tx.Bucket(metaBucket).Get(blockNumberKey)
		if val == nil {
			return tx.Bucket(metaBucket).Put(blockNumberKey, util.Uint64ToBytes(0))
		}
		return nil
	})
}

func (s *BoltBillStore) getMaxBillOrderNumber(tx *bolt.Tx, pubKey []byte) uint64 {
	billOrderNumberBytes := tx.Bucket(metaBucket).Get(pubKey)
	if billOrderNumberBytes != nil {
		return util.BytesToUint64(billOrderNumberBytes)
	}
	return 0
}
