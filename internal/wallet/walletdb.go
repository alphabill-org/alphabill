package wallet

import (
	"alphabill-wallet-sdk/internal/errors"
	"alphabill-wallet-sdk/internal/util"
	"encoding/binary"
	"encoding/json"
	"github.com/holiman/uint256"
	bolt "go.etcd.io/bbolt"
)

var (
	keysBucket  = []byte("keys")
	billsBucket = []byte("bills")
	metaBucket  = []byte("meta")
)

var (
	keyKey         = []byte("key")
	blockHeightKey = []byte("blockHeight")
)

var (
	errWalletDbAlreadyExists    = errors.New("wallet db already exists")
	errKeyNotFound              = errors.New("key not found in wallet")
	errBillWithMinValueNotFound = errors.New("spendable bill with min value not found")
	errBlockHeightNotFound      = errors.New("block height not found")
)

const walletFileName = "wallet.db"

type Db struct {
	db *bolt.DB
}

func CreateNewDb(path string) (*Db, error) {
	dbFilePath := path + walletFileName
	if util.FileExists(dbFilePath) {
		return nil, errWalletDbAlreadyExists
	}
	return OpenDb(path)
}

func OpenDb(path string) (*Db, error) {
	dbFilePath := path + walletFileName
	db, err := bolt.Open(dbFilePath, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}
	return &Db{db}, nil
}

func (d *Db) CreateBuckets() error {
	return d.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(keysBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(billsBucket)
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

func (d *Db) SetKey(key *key) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(keyKey, val)
	})
}

func (d *Db) GetKey() (*key, error) {
	var key *key
	err := d.db.View(func(tx *bolt.Tx) error {
		k := tx.Bucket(keysBucket).Get(keyKey)
		if k == nil {
			return errKeyNotFound
		}
		return json.Unmarshal(k, &key)
	})
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (d *Db) SetBill(bill *bill) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		billId := bill.Id.Bytes32()
		return tx.Bucket(billsBucket).Put(billId[:], val)
	})
}

func (d *Db) ContainsBill(id *uint256.Int) bool {
	res := false
	err := d.db.View(func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		res = tx.Bucket(billsBucket).Get(billId[:]) != nil
		return nil
	})
	if err != nil {
		return false // ignore error
	}
	return res
}

func (d *Db) RemoveBill(id *uint256.Int) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(keysBucket).Delete(id.Bytes())
	})
}

func (d *Db) GetBillWithMinValue(minVal uint64) (*bill, error) {
	var minValBill *bill
	err := d.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(billsBucket).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var b *bill
			err := json.Unmarshal(v, &b)
			if err != nil {
				return nil
			}
			if b.Value >= minVal {
				minValBill = b
				return nil
			}
		}
		return errBillWithMinValueNotFound
	})
	if err != nil {
		return nil, err
	}
	return minValBill, nil
}

func (d *Db) GetBalance() uint64 {
	sum := uint64(0)
	err := d.db.View(func(tx *bolt.Tx) error {
		err := tx.Bucket(billsBucket).ForEach(func(k, v []byte) error {
			var b *bill
			err := json.Unmarshal(v, &b)
			if err != nil {
				return err
			}
			sum += b.Value
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0
	}
	return sum
}

func (d *Db) GetBlockHeight() uint64 {
	blockHeight := uint64(0)
	err := d.db.View(func(tx *bolt.Tx) error {
		blockHeightBytes := tx.Bucket(metaBucket).Get(blockHeightKey)
		if blockHeightBytes == nil {
			return errBlockHeightNotFound
		}
		blockHeight = binary.BigEndian.Uint64(blockHeightBytes)
		return nil
	})
	if err != nil {
		return 0
	}
	return blockHeight
}

func (d *Db) SetBlockHeight(blockHeight uint64) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, blockHeight)
		return tx.Bucket(metaBucket).Put(blockHeightKey, b)
	})
}

func (d *Db) Path() string {
	return d.db.Path()
}

func (d *Db) Close() {
	err := d.db.Close()
	if err != nil {
		// ignore error
	}
}
