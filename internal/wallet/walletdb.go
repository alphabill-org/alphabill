package wallet

import (
	"alphabill-wallet-sdk/internal/errors"
	"alphabill-wallet-sdk/internal/util"
	"encoding/binary"
	"encoding/json"
	"github.com/holiman/uint256"
	bolt "go.etcd.io/bbolt"
	"os"
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

type Db interface {
	CreateBuckets() error
	SetKey(key *key) error
	GetKey() (*key, error)
	SetBill(bill *bill) error
	ContainsBill(id *uint256.Int) (bool, error)
	RemoveBill(id *uint256.Int) error
	GetBillWithMinValue(minVal uint64) (*bill, error)
	GetBalance() (uint64, error)
	GetBlockHeight() (uint64, error)
	SetBlockHeight(blockHeight uint64) error
	Close()
}

type WDb struct {
	db *bolt.DB
}

func CreateNewDb() (*WDb, error) {
	walletDir, err := getWalletDir()
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(walletDir, 0700) // -rwx------
	if err != nil {
		return nil, err
	}

	dbFilePath := walletDir + walletFileName
	if util.FileExists(dbFilePath) {
		return nil, errWalletDbAlreadyExists
	}
	return OpenDb()
}

func OpenDb() (*WDb, error) {
	walletDir, err := getWalletDir()
	if err != nil {
		return nil, err
	}
	dbFilePath := walletDir + walletFileName
	db, err := bolt.Open(dbFilePath, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}
	return &WDb{db}, nil
}

func getWalletDir() (string, error) {
	// TODO add wallet dir to configuration parameter
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	walletDir := homeDir + string(os.PathSeparator) + ".alphabill" + string(os.PathSeparator)
	return walletDir, nil
}

func (d *WDb) CreateBuckets() error {
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

func (d *WDb) SetKey(key *key) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(keyKey, val)
	})
}

func (d *WDb) GetKey() (*key, error) {
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

func (d *WDb) SetBill(bill *bill) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		billId := bill.Id.Bytes32()
		return tx.Bucket(billsBucket).Put(billId[:], val)
	})
}

func (d *WDb) ContainsBill(id *uint256.Int) (bool, error) {
	res := false
	err := d.db.View(func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		res = tx.Bucket(billsBucket).Get(billId[:]) != nil
		return nil
	})
	if err != nil {
		return false, err
	}
	return res, nil
}

func (d *WDb) RemoveBill(id *uint256.Int) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(keysBucket).Delete(id.Bytes())
	})
}

func (d *WDb) GetBillWithMinValue(minVal uint64) (*bill, error) {
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

func (d *WDb) GetBalance() (uint64, error) {
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
		return 0, err
	}
	return sum, nil
}

func (d *WDb) GetBlockHeight() (uint64, error) {
	blockHeight := uint64(0)
	err := d.db.View(func(tx *bolt.Tx) error {
		blockHeightBytes := tx.Bucket(metaBucket).Get(blockHeightKey)
		if blockHeightBytes == nil {
			return errBlockHeightNotFound
		}
		blockHeight = binary.BigEndian.Uint64(blockHeightBytes)
		return nil
	})
	if err != nil && err != errBlockHeightNotFound {
		return 0, err
	}
	return blockHeight, nil
}

func (d *WDb) SetBlockHeight(blockHeight uint64) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, blockHeight)
		return tx.Bucket(metaBucket).Put(blockHeightKey, b)
	})
}

func (d *WDb) Path() string {
	return d.db.Path()
}

func (d *WDb) Close() {
	err := d.db.Close()
	if err != nil {
		// ignore error
	}
}
