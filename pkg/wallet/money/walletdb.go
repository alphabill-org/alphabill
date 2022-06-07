package money

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	bolt "go.etcd.io/bbolt"
)

var (
	keysBucket   = []byte("keys")
	billsBucket  = []byte("bills")
	metaBucket   = []byte("meta")
	dcMetaBucket = []byte("dcMetadata")
)

var (
	accountKeyName     = []byte("accountKey")
	masterKeyName      = []byte("masterKey")
	mnemonicKeyName    = []byte("mnemonicKey")
	blockHeightKeyName = []byte("blockHeightKey")
	isEncryptedKeyName = []byte("isEncryptedKey")
)

var (
	errWalletDbAlreadyExists    = errors.New("wallet db already exists")
	errWalletDbDoesNotExists    = errors.New("cannot open wallet db, file does not exist")
	errKeyNotFound              = errors.New("key not found in wallet")
	errBillWithMinValueNotFound = errors.New("spendable bill with min value not found")
)

const walletFileName = "wallet.db"

type Db interface {
	Do() TxContext
	WithTransaction(func(tx TxContext) error) error
	Close()
	DeleteDb()
}

type TxContext interface {
	GetAccountKey() (*wallet.AccountKey, error)
	SetAccountKey(key *wallet.AccountKey) error

	GetMasterKey() (string, error)
	SetMasterKey(masterKey string) error

	GetMnemonic() (string, error)
	SetMnemonic(mnemonic string) error

	IsEncrypted() (bool, error)
	SetEncrypted(encrypted bool) error
	VerifyPassword() (bool, error)

	GetBlockNumber() (uint64, error)
	SetBlockNumber(blockNumber uint64) error

	SetBill(bill *bill) error
	ContainsBill(id *uint256.Int) (bool, error)
	RemoveBill(id *uint256.Int) error
	GetBills() ([]*bill, error)
	GetBillWithMinValue(minVal uint64) (*bill, error)
	GetBalance() (uint64, error)

	GetDcMetadataMap() (map[uint256.Int]*dcMetadata, error)
	GetDcMetadata(nonce []byte) (*dcMetadata, error)
	SetDcMetadata(nonce []byte, dcMetadata *dcMetadata) error
}

type wdb struct {
	db         *bolt.DB
	dbFilePath string
	walletPass string
}

type wdbtx struct {
	wdb *wdb
	tx  *bolt.Tx
}

func OpenDb(config WalletConfig) (*wdb, error) {
	walletDir, err := config.GetWalletDir()
	if err != nil {
		return nil, err
	}
	dbFilePath := path.Join(walletDir, walletFileName)
	return openDb(dbFilePath, config.WalletPass, false)
}

func (w *wdbtx) SetAccountKey(key *wallet.AccountKey) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		val, err = w.encryptValue(val)
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(accountKeyName, val)
	}, true)
}

func (w *wdbtx) GetAccountKey() (*wallet.AccountKey, error) {
	var key *wallet.AccountKey
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		k := tx.Bucket(keysBucket).Get(accountKeyName)
		if k == nil {
			return errKeyNotFound
		}
		val, err := w.decryptValue(k)
		if err != nil {
			return err
		}
		err = json.Unmarshal(val, &key)
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

func (w *wdbtx) SetMasterKey(masterKey string) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		val, err := w.encryptValue([]byte(masterKey))
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(masterKeyName, val)
	}, true)
}

func (w *wdbtx) GetMasterKey() (string, error) {
	var res string
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		masterKey := tx.Bucket(keysBucket).Get(masterKeyName)
		if masterKey == nil {
			return errKeyNotFound
		}
		val, err := w.decryptValue(masterKey)
		if err != nil {
			return err
		}
		res = string(val)
		return nil
	}, false)
	if err != nil {
		return "", err
	}
	return res, nil
}

func (w *wdbtx) SetMnemonic(mnemonic string) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		val, err := w.encryptValue([]byte(mnemonic))
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(mnemonicKeyName, val)
	}, true)
}

func (w *wdbtx) GetMnemonic() (string, error) {
	var res string
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		mnemonic := tx.Bucket(keysBucket).Get(mnemonicKeyName)
		if mnemonic == nil {
			return errKeyNotFound
		}
		val, err := w.decryptValue(mnemonic)
		if err != nil {
			return err
		}
		res = string(val)
		return nil
	}, false)
	if err != nil {
		return "", err
	}
	return res, nil
}

func (w *wdbtx) SetEncrypted(encrypted bool) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		var b byte
		if encrypted {
			b = 0x01
		} else {
			b = 0x00
		}
		return tx.Bucket(metaBucket).Put(isEncryptedKeyName, []byte{b})
	}, true)
}

func (w *wdbtx) IsEncrypted() (bool, error) {
	var res bool
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		encrypted := tx.Bucket(metaBucket).Get(isEncryptedKeyName)
		res = bytes.Equal(encrypted, []byte{0x01})
		return nil
	}, false)
	if err != nil {
		return false, err
	}
	return res, nil
}

func (w *wdbtx) VerifyPassword() (bool, error) {
	encrypted, err := w.IsEncrypted()
	if err != nil {
		return false, err
	}
	if encrypted {
		_, err = w.GetAccountKey()
		if err != nil {
			if errors.Is(err, abcrypto.ErrEmptyPassphrase) {
				return false, nil
			}
			if strings.Contains(err.Error(), abcrypto.ErrMsgDecryptingValue) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

func (w *wdbtx) SetBill(bill *bill) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("adding bill: value=%d id=%s", bill.Value, bill.Id.String()))
		return tx.Bucket(billsBucket).Put(bill.getId(), val)
	}, true)
}

func (w *wdbtx) ContainsBill(id *uint256.Int) (bool, error) {
	var res bool
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		res = tx.Bucket(billsBucket).Get(billId[:]) != nil
		return nil
	}, false)
	if err != nil {
		return false, err
	}
	return res, nil
}

func (w *wdbtx) GetBills() ([]*bill, error) {
	var res []*bill
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
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

func (w *wdbtx) RemoveBill(id *uint256.Int) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		bytes32 := id.Bytes32()
		return tx.Bucket(billsBucket).Delete(bytes32[:])
	}, true)
}

func (w *wdbtx) GetBillWithMinValue(minVal uint64) (*bill, error) {
	var res *bill
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
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

func (w *wdbtx) GetBalance() (uint64, error) {
	sum := uint64(0)
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
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

func (w *wdbtx) GetBlockNumber() (uint64, error) {
	var res uint64
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
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

func (w *wdbtx) SetBlockNumber(blockHeight uint64) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, blockHeight)
		return tx.Bucket(metaBucket).Put(blockHeightKeyName, b)
	}, true)
}

func (w *wdbtx) GetDcMetadataMap() (map[uint256.Int]*dcMetadata, error) {
	res := map[uint256.Int]*dcMetadata{}
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(dcMetaBucket).ForEach(func(k, v []byte) error {
			var m *dcMetadata
			err := json.Unmarshal(v, &m)
			if err != nil {
				return err
			}
			res[*uint256.NewInt(0).SetBytes(k)] = m
			return nil
		})
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (w *wdbtx) GetDcMetadata(nonce []byte) (*dcMetadata, error) {
	var res *dcMetadata
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		m := tx.Bucket(dcMetaBucket).Get(nonce)
		if m != nil {
			return json.Unmarshal(m, &res)
		}
		return nil
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (w *wdbtx) SetDcMetadata(dcNonce []byte, dcMetadata *dcMetadata) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		if dcMetadata != nil {
			val, err := json.Marshal(dcMetadata)
			if err != nil {
				return err
			}
			return tx.Bucket(dcMetaBucket).Put(dcNonce, val)
		}
		return tx.Bucket(dcMetaBucket).Delete(dcNonce)
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

func (w *wdb) WithTransaction(fn func(txc TxContext) error) error {
	return w.db.Update(func(tx *bolt.Tx) error {
		return fn(&wdbtx{wdb: w, tx: tx})
	})
}

func (w *wdb) Do() TxContext {
	return &wdbtx{wdb: w, tx: nil}
}

func (w *wdb) Path() string {
	return w.dbFilePath
}

func (w *wdb) Close() {
	if w.db == nil {
		return
	}
	log.Info("closing wallet db")
	err := w.db.Close()
	if err != nil {
		log.Warning("error closing db: ", err)
	}
}

func (w *wdbtx) withTx(dbTx *bolt.Tx, myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if dbTx != nil {
		return myFunc(dbTx)
	} else if writeTx {
		return w.wdb.db.Update(myFunc)
	} else {
		return w.wdb.db.View(myFunc)
	}
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
		_, err = tx.CreateBucketIfNotExists(dcMetaBucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func openDb(dbFilePath string, walletPass string, create bool) (*wdb, error) {
	exists := util.FileExists(dbFilePath)
	if create && exists {
		return nil, errWalletDbAlreadyExists
	} else if !create && !exists {
		return nil, errWalletDbDoesNotExists
	}

	db, err := bolt.Open(dbFilePath, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}

	w := &wdb{db, dbFilePath, walletPass}
	err = w.createBuckets()
	if err != nil {
		return nil, err
	}
	return w, nil
}

func createNewDb(config WalletConfig) (*wdb, error) {
	walletDir, err := config.GetWalletDir()
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(walletDir, 0700) // -rwx------
	if err != nil {
		return nil, err
	}

	dbFilePath := path.Join(walletDir, walletFileName)
	return openDb(dbFilePath, config.WalletPass, true)
}

func parseBill(v []byte) (*bill, error) {
	var b *bill
	err := json.Unmarshal(v, &b)
	return b, err
}

func (w *wdbtx) encryptValue(val []byte) ([]byte, error) {
	isEncrypted, err := w.IsEncrypted()
	if err != nil {
		return nil, err
	}
	if !isEncrypted {
		return val, nil
	}
	encryptedValue, err := crypto.Encrypt(w.wdb.walletPass, val)
	if err != nil {
		return nil, err
	}
	return []byte(encryptedValue), nil
}

func (w *wdbtx) decryptValue(val []byte) ([]byte, error) {
	isEncrypted, err := w.IsEncrypted()
	if err != nil {
		return nil, err
	}
	if !isEncrypted {
		return val, nil
	}
	decryptedValue, err := crypto.Decrypt(w.wdb.walletPass, string(val))
	if err != nil {
		return nil, err
	}
	return decryptedValue, nil
}
