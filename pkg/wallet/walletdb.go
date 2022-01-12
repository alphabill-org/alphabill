package wallet

import (
	"alphabill-wallet-sdk/internal/errors"
	"alphabill-wallet-sdk/internal/util"
	"alphabill-wallet-sdk/pkg/log"
	"encoding/binary"
	"encoding/json"
	"github.com/holiman/uint256"
	bolt "go.etcd.io/bbolt"
	"os"
	"path"
)

var (
	keysBucket    = []byte("keys")
	billsBucket   = []byte("bills")
	dcBillsBucket = []byte("dcBills")
	metaBucket    = []byte("meta")
)

var (
	accountKeyName       = []byte("accountKey")
	masterKeyName        = []byte("masterKey")
	mnemonicKeyName      = []byte("mnemonicKey")
	blockHeightKeyName   = []byte("blockHeightKey")
	dcBlockHeightKeyName = []byte("dcBlockHeightKey")
	dcValueSumKeyName    = []byte("dcValueSumKey")
)

var (
	errWalletDbAlreadyExists    = errors.New("wallet db already exists")
	errWalletDbDoesNotExists    = errors.New("cannot open wallet db, file does not exits")
	errKeyNotFound              = errors.New("key not found in wallet")
	errBillWithMinValueNotFound = errors.New("spendable bill with min value not found")
	errBlockHeightNotFound      = errors.New("block height not found")
)

const walletFileName = "wallet.db"

type Db interface {
	SetAccountKey(key *accountKey) error
	GetAccountKey() (*accountKey, error)
	SetMasterKey(string) error
	GetMasterKey() (string, error)
	SetMnemonic(string) error
	GetMnemonic() (string, error)
	SetBill(bill *bill) error
	GetBill(id *uint256.Int) (*bill, error)
	ContainsBill(id *uint256.Int) (bool, error)
	RemoveBill(id *uint256.Int) error
	GetBills() ([]*bill, error)
	GetBillWithMinValue(minVal uint64) (*bill, error)
	GetBalance() (uint64, error)
	GetBlockHeight() (uint64, error)
	SetBlockHeight(blockHeight uint64) error
	Close()
	DeleteDb()
	SetDcBlockHeight(blockHeight uint64) error
	SetDcValueSum(dcValueSum uint64) error
	GetDcBlockHeight() (uint64, error)
	GetDcValueSum() (uint64, error)
}

type wdb struct {
	db         *bolt.DB
	dbFilePath string
}

func OpenDb(config *Config) (*wdb, error) {
	dbFilePath := path.Join(config.DbPath, walletFileName)
	return openDb(dbFilePath, false)
}

func openDb(dbFilePath string, create bool) (*wdb, error) {
	if create {
		if util.FileExists(dbFilePath) {
			return nil, errWalletDbAlreadyExists
		}
	} else {
		if !util.FileExists(dbFilePath) {
			return nil, errWalletDbDoesNotExists
		}
	}

	db, err := bolt.Open(dbFilePath, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}

	w := &wdb{db, dbFilePath}
	err = w.createBuckets()
	if err != nil {
		w.DeleteDb()
		return nil, err
	}
	return w, nil
}

func createNewDb(config *Config) (*wdb, error) {
	walletDir, err := config.GetWalletDir()
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(walletDir, 0700) // -rwx------
	if err != nil {
		return nil, err
	}

	dbFilePath := path.Join(walletDir, walletFileName)
	return openDb(dbFilePath, true)
}

func (d *wdb) createBuckets() error {
	return d.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(keysBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(billsBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(dcBillsBucket)
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

func (d *wdb) SetAccountKey(key *accountKey) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(accountKeyName, val)
	})
}

func (d *wdb) GetAccountKey() (*accountKey, error) {
	var key *accountKey
	err := d.db.View(func(tx *bolt.Tx) error {
		k := tx.Bucket(keysBucket).Get(accountKeyName)
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

func (d *wdb) SetMasterKey(masterKey string) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(keysBucket).Put(masterKeyName, []byte(masterKey))
	})
}

func (d *wdb) GetMasterKey() (string, error) {
	var masterKey []byte
	err := d.db.View(func(tx *bolt.Tx) error {
		masterKey = tx.Bucket(keysBucket).Get(masterKeyName)
		if masterKey == nil {
			return errKeyNotFound
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return string(masterKey), nil
}

func (d *wdb) SetMnemonic(mnemonic string) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(keysBucket).Put(mnemonicKeyName, []byte(mnemonic))
	})
}

func (d *wdb) GetMnemonic() (string, error) {
	var mnemonic []byte
	err := d.db.View(func(tx *bolt.Tx) error {
		mnemonic = tx.Bucket(keysBucket).Get(mnemonicKeyName)
		if mnemonic == nil {
			return errKeyNotFound
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return string(mnemonic), nil
}

func (d *wdb) SetBill(bill *bill) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		val, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		return tx.Bucket(billsBucket).Put(bill.getId(), val)
	})
}

func (d *wdb) GetBill(id *uint256.Int) (*bill, error) {
	var b *bill
	err := d.db.View(func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		billBytes := tx.Bucket(billsBucket).Get(billId[:])
		if billBytes != nil {
			return json.Unmarshal(billBytes, &b)
		}
		return nil
	})
	return b, err
}

func (d *wdb) ContainsBill(id *uint256.Int) (bool, error) {
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

func (d *wdb) GetBills() ([]*bill, error) {
	var res []*bill
	err := d.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(billsBucket)
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			b, err := parseBill(v)
			if err != nil {
				return err
			}
			res = append(res, b)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (d *wdb) RemoveBill(id *uint256.Int) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		bytes32 := id.Bytes32()
		return tx.Bucket(billsBucket).Delete(bytes32[:])
	})
}

func (d *wdb) GetBillWithMinValue(minVal uint64) (*bill, error) {
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

func (d *wdb) GetBalance() (uint64, error) {
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

func (d *wdb) GetBlockHeight() (uint64, error) {
	blockHeight := uint64(0)
	err := d.db.View(func(tx *bolt.Tx) error {
		blockHeightBytes := tx.Bucket(metaBucket).Get(blockHeightKeyName)
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

func (d *wdb) SetBlockHeight(blockHeight uint64) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, blockHeight)
		return tx.Bucket(metaBucket).Put(blockHeightKeyName, b)
	})
}

func (d *wdb) Path() string {
	return d.dbFilePath
}

func (d *wdb) Close() {
	if d.db == nil {
		return
	}
	err := d.db.Close()
	if err != nil {
		log.Warning("error closing db: ", err)
	}
}

func (d *wdb) GetDcBlockHeight() (uint64, error) {
	dcBlockHeight := uint64(0)
	err := d.db.View(func(tx *bolt.Tx) error {
		dcBlockHeightBytes := tx.Bucket(metaBucket).Get(dcBlockHeightKeyName)
		if dcBlockHeightBytes != nil {
			dcBlockHeight = binary.BigEndian.Uint64(dcBlockHeightBytes)
		}
		return nil
	})
	return dcBlockHeight, err
}

func (d *wdb) SetDcBlockHeight(dcBlockHeight uint64) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, dcBlockHeight)
		return tx.Bucket(metaBucket).Put(dcBlockHeightKeyName, b)
	})
}

func (d *wdb) GetDcValueSum() (uint64, error) {
	dcValueSum := uint64(0)
	err := d.db.View(func(tx *bolt.Tx) error {
		dcValueSumBytes := tx.Bucket(metaBucket).Get(dcValueSumKeyName)
		if dcValueSumBytes != nil {
			dcValueSum = binary.BigEndian.Uint64(dcValueSumBytes)
		}
		return nil
	})
	return dcValueSum, err
}

func (d *wdb) SetDcValueSum(dcValueSum uint64) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, dcValueSum)
		return tx.Bucket(metaBucket).Put(dcValueSumKeyName, b)
	})
}

func (d *wdb) DeleteDb() {
	if d.db == nil {
		return
	}
	errClose := d.db.Close()
	if errClose != nil {
		log.Warning("error closing db: ", errClose)
	}
	errRemove := os.Remove(d.dbFilePath)
	if errRemove != nil {
		log.Warning("error removing db: ", errRemove)
	}
}

func parseBill(v []byte) (*bill, error) {
	var b *bill
	err := json.Unmarshal(v, &b)
	return b, err
}
