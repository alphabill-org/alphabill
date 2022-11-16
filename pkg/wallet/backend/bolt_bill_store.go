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
	billsBucket  = []byte("billsBucket")  // pubkey => bucket[bill_id]=bill
	proofsBucket = []byte("proofsBucket") // bill_id => block_proof
	keysBucket   = []byte("keysBucket")   // pubkey => hashed pubkey
	metaBucket   = []byte("metaBucket")   // block_number_key => block_number_val; pubkey => pubkey_block_order_number

	blockNumberKey = []byte("blockNumberKey")
)

var (
	ErrKeyAlreadyExists = errors.New("key already exists")
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
		pubkeyBillsBucket := tx.Bucket(billsBucket).Bucket(pubkey)
		if pubkeyBillsBucket == nil {
			return nil
		}
		return pubkeyBillsBucket.ForEach(func(billId, billBytes []byte) error {
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

func (s *BoltBillStore) AddBill(pubKey []byte, b *Bill) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		pubkeyBillsBucket, err := tx.Bucket(billsBucket).CreateBucketIfNotExists(pubKey)
		if err != nil {
			return err
		}
		billOrderNumber := s.getMaxBillOrderNumber(tx, pubKey)
		b.OrderNumber = billOrderNumber + 1
		billBytes, err := json.Marshal(b)
		if err != nil {
			return err
		}
		err = tx.Bucket(metaBucket).Put(pubKey, util.Uint64ToBytes(b.OrderNumber))
		if err != nil {
			return err
		}
		return pubkeyBillsBucket.Put(b.Id, billBytes)
	})
}

func (s *BoltBillStore) AddBillWithProof(pubKey []byte, b *Bill, proof *BlockProof) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		pubkeyBillsBucket, err := tx.Bucket(billsBucket).CreateBucketIfNotExists(pubKey)
		if err != nil {
			return err
		}
		billOrderNumber := s.getMaxBillOrderNumber(tx, pubKey)
		b.OrderNumber = billOrderNumber + 1
		billBytes, err := json.Marshal(b)
		if err != nil {
			return err
		}
		err = pubkeyBillsBucket.Put(b.Id[:], billBytes)
		if err != nil {
			return err
		}
		val, err := json.Marshal(proof)
		if err != nil {
			return err
		}
		err = tx.Bucket(metaBucket).Put(pubKey, util.Uint64ToBytes(b.OrderNumber))
		if err != nil {
			return err
		}
		return tx.Bucket(proofsBucket).Put(b.Id, val)
	})
}

func (s *BoltBillStore) RemoveBill(pubKey []byte, id []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		pubkeyBillsBucket := tx.Bucket(billsBucket).Bucket(pubKey)
		if pubkeyBillsBucket == nil {
			return nil
		}
		return pubkeyBillsBucket.Delete(id)
	})
}

func (s *BoltBillStore) ContainsBill(pubKey []byte, id []byte) (bool, error) {
	res := false
	err := s.db.View(func(tx *bolt.Tx) error {
		pubkeyBillsBucket := tx.Bucket(billsBucket).Bucket(pubKey)
		if pubkeyBillsBucket == nil {
			return nil
		}
		billBytes := pubkeyBillsBucket.Get(id)
		if len(billBytes) > 0 {
			res = true
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return res, nil
}

func (s *BoltBillStore) GetBlockProof(billId []byte) (*BlockProof, error) {
	var proof *BlockProof
	err := s.db.View(func(tx *bolt.Tx) error {
		proofBytes := tx.Bucket(proofsBucket).Get(billId)
		if proofBytes == nil {
			return nil
		}
		return json.Unmarshal(proofBytes, &proof)
	})
	if err != nil {
		return nil, err
	}
	return proof, nil
}

func (s *BoltBillStore) SetBlockProof(proof *BlockProof) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(proof)
		if err != nil {
			return err
		}
		return tx.Bucket(proofsBucket).Put(proof.BillId, val)
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
		_, err := tx.CreateBucketIfNotExists(billsBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(proofsBucket)
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
