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
	keysBucket  = []byte("keys")
	billsBucket = []byte("bills")
	metaBucket  = []byte("meta")
)

var (
	accountKeyName     = []byte("accountKey")
	masterKeyName      = []byte("masterKey")
	mnemonicKeyName    = []byte("mnemonicKey")
	blockHeightKeyName = []byte("blockHeightKey")
	dcTimeoutKeyName   = []byte("dcTimeout")
	swapTimeoutKeyName = []byte("swapTimeout")
	dcValueSumKeyName  = []byte("dcValueSumKey")
)

var (
	errWalletDbAlreadyExists    = errors.New("wallet db already exists")
	errWalletDbDoesNotExists    = errors.New("cannot open wallet db, file does not exits")
	errKeyNotFound              = errors.New("key not found in wallet")
	errBillWithMinValueNotFound = errors.New("spendable bill with min value not found")
)

const walletFileName = "wallet.db"

type Db interface {
	GetAccountKey() (*accountKey, error)
	SetAccountKey(key *accountKey) error

	GetMasterKey() (string, error)
	SetMasterKey(string) error

	GetMnemonic() (string, error)
	SetMnemonic(string) error

	GetBill(id *uint256.Int) (*bill, error)
	SetBill(bill *bill) error
	ContainsBill(id *uint256.Int) (bool, error)
	RemoveBill(id *uint256.Int) error
	GetBills() ([]*bill, error)
	GetBillWithMinValue(minVal uint64) (*bill, error)
	GetBalance() (uint64, error)

	GetBlockHeight() (uint64, error)
	SetBlockHeight(blockHeight uint64) error

	GetDcTimeout() (uint64, error)
	SetDcTimeout(blockHeight uint64) error

	GetSwapTimeout() (uint64, error)
	SetSwapTimeout(blockHeight uint64) error

	GetDcValueSum() (uint64, error)
	SetDcValueSum(dcValueSum uint64) error

	WithTransaction(func() error) error

	Close()
	DeleteDb()
}

type wdb struct {
	db         *bolt.DB
	dbFilePath string
	tx         *bolt.Tx
}

func OpenDb(config *Config) (*wdb, error) {
	dbFilePath := path.Join(config.DbPath, walletFileName)
	return openDb(dbFilePath, false)
}

func (w *wdb) SetAccountKey(key *accountKey) error {
	return w.withTx(func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(accountKeyName, val)
	}, true)
}

func (w *wdb) GetAccountKey() (*accountKey, error) {
	var key *accountKey
	err := w.withTx(func(tx *bolt.Tx) error {
		k := tx.Bucket(keysBucket).Get(accountKeyName)
		if k == nil {
			return errKeyNotFound
		}
		err := json.Unmarshal(k, &key)
		if err != nil {
			return err
		}
		return nil
	}, false)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (w *wdb) SetMasterKey(masterKey string) error {
	return w.withTx(func(tx *bolt.Tx) error {
		return tx.Bucket(keysBucket).Put(masterKeyName, []byte(masterKey))
	}, true)
}

func (w *wdb) GetMasterKey() (string, error) {
	var res string
	err := w.withTx(func(tx *bolt.Tx) error {
		masterKey := tx.Bucket(keysBucket).Get(masterKeyName)
		if masterKey == nil {
			return errKeyNotFound
		}
		res = string(masterKey)
		return nil
	}, false)
	if err != nil {
		return "", err
	}
	return res, nil
}

func (w *wdb) SetMnemonic(mnemonic string) error {
	return w.withTx(func(tx *bolt.Tx) error {
		return tx.Bucket(keysBucket).Put(mnemonicKeyName, []byte(mnemonic))
	}, true)
}

func (w *wdb) GetMnemonic() (string, error) {
	var res string
	err := w.withTx(func(tx *bolt.Tx) error {
		mnemonic := tx.Bucket(keysBucket).Get(mnemonicKeyName)
		if mnemonic == nil {
			return errKeyNotFound
		}
		res = string(mnemonic)
		return nil
	}, false)
	if err != nil {
		return "", err
	}
	return res, nil
}

func (w *wdb) SetBill(bill *bill) error {
	return w.withTx(func(tx *bolt.Tx) error {
		val, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		return tx.Bucket(billsBucket).Put(bill.getId(), val)
	}, true)
}

func (w *wdb) GetBill(id *uint256.Int) (*bill, error) {
	var b *bill
	err := w.withTx(func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		billBytes := tx.Bucket(billsBucket).Get(billId[:])
		if billBytes != nil {
			return json.Unmarshal(billBytes, &b)
		}
		return nil
	}, false)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (w *wdb) ContainsBill(id *uint256.Int) (bool, error) {
	var res bool
	err := w.withTx(func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		res = tx.Bucket(billsBucket).Get(billId[:]) != nil
		return nil
	}, false)
	if err != nil {
		return false, err
	}
	return res, nil
}

func (w *wdb) GetBills() ([]*bill, error) {
	var res []*bill
	err := w.withTx(func(tx *bolt.Tx) error {
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
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (w *wdb) RemoveBill(id *uint256.Int) error {
	return w.withTx(func(tx *bolt.Tx) error {
		bytes32 := id.Bytes32()
		return tx.Bucket(billsBucket).Delete(bytes32[:])
	}, true)
}

func (w *wdb) GetBillWithMinValue(minVal uint64) (*bill, error) {
	var res *bill
	err := w.withTx(func(tx *bolt.Tx) error {
		c := tx.Bucket(billsBucket).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var b *bill
			err := json.Unmarshal(v, &b)
			if err != nil {
				return err
			}
			if b.Value >= minVal {
				res = b
				return nil
			}
		}
		return errBillWithMinValueNotFound
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (w *wdb) GetBalance() (uint64, error) {
	sum := uint64(0)
	err := w.withTx(func(tx *bolt.Tx) error {
		return tx.Bucket(billsBucket).ForEach(func(k, v []byte) error {
			var b *bill
			err := json.Unmarshal(v, &b)
			if err != nil {
				return err
			}
			sum += b.Value
			return nil
		})
	}, false)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

func (w *wdb) GetBlockHeight() (uint64, error) {
	var res uint64
	err := w.withTx(func(tx *bolt.Tx) error {
		blockHeightBytes := tx.Bucket(metaBucket).Get(blockHeightKeyName)
		if blockHeightBytes == nil {
			return nil
		}
		res = binary.BigEndian.Uint64(blockHeightBytes)
		return nil
	}, false)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (w *wdb) SetBlockHeight(blockHeight uint64) error {
	return w.withTx(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, blockHeight)
		return tx.Bucket(metaBucket).Put(blockHeightKeyName, b)
	}, true)
}

func (w *wdb) GetDcTimeout() (uint64, error) {
	var res uint64
	err := w.withTx(func(tx *bolt.Tx) error {
		b := tx.Bucket(metaBucket).Get(dcTimeoutKeyName)
		if b != nil {
			res = binary.BigEndian.Uint64(b)
		}
		return nil
	}, false)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (w *wdb) SetDcTimeout(dcBlockHeight uint64) error {
	return w.withTx(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, dcBlockHeight)
		return tx.Bucket(metaBucket).Put(dcTimeoutKeyName, b)
	}, true)
}

func (w *wdb) GetSwapTimeout() (uint64, error) {
	var res uint64
	err := w.withTx(func(tx *bolt.Tx) error {
		b := tx.Bucket(metaBucket).Get(swapTimeoutKeyName)
		if b != nil {
			res = binary.BigEndian.Uint64(b)
		}
		return nil
	}, false)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (w *wdb) SetSwapTimeout(timeout uint64) error {
	return w.withTx(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, timeout)
		return tx.Bucket(metaBucket).Put(swapTimeoutKeyName, b)
	}, true)
}

func (w *wdb) GetDcValueSum() (uint64, error) {
	var res uint64
	err := w.withTx(func(tx *bolt.Tx) error {
		b := tx.Bucket(metaBucket).Get(dcValueSumKeyName)
		if b != nil {
			res = binary.BigEndian.Uint64(b)
		}
		return nil
	}, false)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (w *wdb) SetDcValueSum(dcValueSum uint64) error {
	return w.withTx(func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, dcValueSum)
		return tx.Bucket(metaBucket).Put(dcValueSumKeyName, b)
	}, true)
}

func (w *wdb) DeleteDb() {
	if w.db == nil {
		return
	}
	errClose := w.db.Close()
	if errClose != nil {
		log.Warning("error closing db: ", errClose)
	}
	errRemove := os.Remove(w.dbFilePath)
	if errRemove != nil {
		log.Warning("error removing db: ", errRemove)
	}
}

func (w *wdb) WithTransaction(myFunc func() error) error {
	return w.db.Update(func(tx *bolt.Tx) error {
		w.tx = tx
		defer w.clearTx()
		return myFunc()
	})
}

func (w *wdb) Path() string {
	return w.dbFilePath
}

func (w *wdb) Close() {
	if w.db == nil {
		return
	}
	err := w.db.Close()
	if err != nil {
		log.Warning("error closing db: ", err)
	}
}

func (w *wdb) withTx(myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if w.tx != nil {
		return myFunc(w.tx)
	} else if writeTx {
		return w.db.Update(myFunc)
	} else {
		return w.db.View(myFunc)
	}
}

func (w *wdb) clearTx() {
	w.tx = nil
}

func (w *wdb) createBuckets() error {
	return w.db.Update(func(tx *bolt.Tx) error {
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

	w := &wdb{db, dbFilePath, nil}
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

func parseBill(v []byte) (*bill, error) {
	var b *bill
	err := json.Unmarshal(v, &b)
	return b, err
}
