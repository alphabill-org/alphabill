package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"

	"github.com/alphabill-org/alphabill/internal/crypto"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	bolt "go.etcd.io/bbolt"
)

var (
	keysBucket     = []byte("keys")
	accountsBucket = []byte("accounts")
	metaBucket     = []byte("meta")

	masterKeyName          = []byte("masterKey")
	mnemonicKeyName        = []byte("mnemonicKey")
	accountKeyName         = []byte("accountKey")
	isEncryptedKeyName     = []byte("isEncryptedKey")
	maxAccountIndexKeyName = []byte("maxAccountIndexKey")

	errAccountDbAlreadyExists = errors.New("account db already exists")
	errAccountDbDoesNotExists = errors.New("cannot open Account db, file does not exist")
	errAccountNotFound        = errors.New("account does not exist")
)

const AccountFileName = "accounts.db"

type Db interface {
	Do() TxContext
	WithTransaction(func(tx TxContext) error) error
	Close()
	DeleteDb()
}

type TxContext interface {
	AddAccount(accountIndex uint64, key *wallet.AccountKey) error
	GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error)
	GetAccountKeys() ([]*wallet.AccountKey, error)
	GetMaxAccountIndex() (uint64, error)
	SetMaxAccountIndex(accountIndex uint64) error

	GetMasterKey() (string, error)
	SetMasterKey(masterKey string) error

	GetMnemonic() (string, error)
	SetMnemonic(mnemonic string) error

	IsEncrypted() (bool, error)
	SetEncrypted(encrypted bool) error
	VerifyPassword() (bool, error)
}

type adb struct {
	db         *bolt.DB
	dbFilePath string
	password   string
}

type adbtx struct {
	adb *adb
	tx  *bolt.Tx
}

func (a *adbtx) AddAccount(accountIndex uint64, key *wallet.AccountKey) error {
	return a.withTx(a.tx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		val, err = a.encryptValue(val)
		if err != nil {
			return err
		}
		accBucket, err := tx.Bucket(accountsBucket).CreateBucketIfNotExists(util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		return accBucket.Put(accountKeyName, val)
	}, true)
}

func (a *adbtx) GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error) {
	var key *wallet.AccountKey
	err := a.withTx(a.tx, func(tx *bolt.Tx) error {
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		k := bkt.Get(accountKeyName)
		val, err := a.decryptValue(k)
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

func (a *adbtx) GetAccountKeys() ([]*wallet.AccountKey, error) {
	keys := make(map[uint64]*wallet.AccountKey)
	err := a.withTx(a.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(accountsBucket).ForEach(func(accountIndex, v []byte) error {
			if v != nil { // v is nil if entry is a bucket (ignore accounts metadata)
				return nil
			}
			accountBucket, err := getAccountBucket(tx, accountIndex)
			if err != nil {
				return err
			}
			accountKey := accountBucket.Get(accountKeyName)
			accountKeyDecrypted, err := a.decryptValue(accountKey)
			if err != nil {
				return err
			}
			var accountKeyRes *wallet.AccountKey
			err = json.Unmarshal(accountKeyDecrypted, &accountKeyRes)
			if err != nil {
				return err
			}
			accountIndexUint64 := util.BytesToUint64(accountIndex)
			keys[accountIndexUint64] = accountKeyRes
			return nil
		})
	}, false)
	if err != nil {
		return nil, err
	}
	res := make([]*wallet.AccountKey, len(keys))
	for accIdx, key := range keys {
		res[accIdx] = key
	}
	return res, nil
}

func (a *adbtx) SetMasterKey(masterKey string) error {
	return a.withTx(a.tx, func(tx *bolt.Tx) error {
		val, err := a.encryptValue([]byte(masterKey))
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(masterKeyName, val)
	}, true)
}

func (a *adbtx) GetMasterKey() (string, error) {
	var res string
	err := a.withTx(a.tx, func(tx *bolt.Tx) error {
		masterKey := tx.Bucket(keysBucket).Get(masterKeyName)
		val, err := a.decryptValue(masterKey)
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

func (a *adbtx) SetMaxAccountIndex(accountIndex uint64) error {
	return a.withTx(a.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(accountsBucket).Put(maxAccountIndexKeyName, util.Uint64ToBytes(accountIndex))
	}, true)
}

func (a *adbtx) GetMaxAccountIndex() (uint64, error) {
	var res uint64
	err := a.withTx(a.tx, func(tx *bolt.Tx) error {
		accountIndex := tx.Bucket(accountsBucket).Get(maxAccountIndexKeyName)
		res = util.BytesToUint64(accountIndex)
		return nil
	}, false)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (a *adbtx) SetMnemonic(mnemonic string) error {
	return a.withTx(a.tx, func(tx *bolt.Tx) error {
		val, err := a.encryptValue([]byte(mnemonic))
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(mnemonicKeyName, val)
	}, true)
}

func (a *adbtx) GetMnemonic() (string, error) {
	var res string
	err := a.withTx(a.tx, func(tx *bolt.Tx) error {
		mnemonic := tx.Bucket(keysBucket).Get(mnemonicKeyName)
		val, err := a.decryptValue(mnemonic)
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

func (a *adbtx) SetEncrypted(encrypted bool) error {
	return a.withTx(a.tx, func(tx *bolt.Tx) error {
		var b byte
		if encrypted {
			b = 0x01
		} else {
			b = 0x00
		}
		return tx.Bucket(metaBucket).Put(isEncryptedKeyName, []byte{b})
	}, true)
}

func (a *adbtx) IsEncrypted() (bool, error) {
	var res bool
	err := a.withTx(a.tx, func(tx *bolt.Tx) error {
		encrypted := tx.Bucket(metaBucket).Get(isEncryptedKeyName)
		res = bytes.Equal(encrypted, []byte{0x01})
		return nil
	}, false)
	if err != nil {
		return false, err
	}
	return res, nil
}

func (a *adbtx) VerifyPassword() (bool, error) {
	encrypted, err := a.IsEncrypted()
	if err != nil {
		return false, err
	}
	if encrypted {
		_, err := a.GetAccountKeys()
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

func getAccountBucket(tx *bolt.Tx, accountIndex []byte) (*bolt.Bucket, error) {
	bkt := tx.Bucket(accountsBucket).Bucket(accountIndex)
	if bkt == nil {
		return nil, errAccountNotFound
	}
	return bkt, nil
}

func (a *adbtx) encryptValue(val []byte) ([]byte, error) {
	isEncrypted, err := a.IsEncrypted()
	if err != nil {
		return nil, err
	}
	if !isEncrypted {
		return val, nil
	}
	encryptedValue, err := crypto.Encrypt(a.adb.password, val)
	if err != nil {
		return nil, err
	}
	return []byte(encryptedValue), nil
}

func (a *adbtx) decryptValue(val []byte) ([]byte, error) {
	isEncrypted, err := a.IsEncrypted()
	if err != nil {
		return nil, err
	}
	if !isEncrypted {
		return val, nil
	}
	decryptedValue, err := crypto.Decrypt(a.adb.password, string(val))
	if err != nil {
		return nil, err
	}
	return decryptedValue, nil
}

func openDb(dbFilePath string, pw string, create bool) (*adb, error) {
	exists := util.FileExists(dbFilePath)
	if create && exists {
		return nil, errAccountDbAlreadyExists
	} else if !create && !exists {
		return nil, errAccountDbDoesNotExists
	}

	db, err := bolt.Open(dbFilePath, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}

	a := &adb{db, dbFilePath, pw}
	err = a.createBuckets()
	if err != nil {
		return nil, err
	}

	if create {
		err := a.Do().SetEncrypted(pw != "")
		if err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (a *adb) Close() {
	if a.db == nil {
		return
	}
	log.Info("closing Account db")
	err := a.db.Close()
	if err != nil {
		log.Warning("error closing db: ", err)
	}
}

func (a *adb) DeleteDb() {
	if a.db == nil {
		return
	}
	errClose := a.db.Close()
	if errClose != nil {
		log.Warning("error closing db: ", errClose)
	}
	errRemove := os.Remove(a.dbFilePath)
	if errRemove != nil {
		log.Warning("error removing db: ", errRemove)
	}
}

func (a *adb) WithTransaction(fn func(txc TxContext) error) error {
	return a.db.Update(func(tx *bolt.Tx) error {
		return fn(&adbtx{adb: a, tx: tx})
	})
}

func (a *adb) Do() TxContext {
	return &adbtx{adb: a, tx: nil}
}

func createNewDb(dir string, pw string) (*adb, error) {
	err := os.MkdirAll(dir, 0700) // -rwx------
	if err != nil {
		return nil, err
	}

	dbFilePath := path.Join(dir, AccountFileName)
	return openDb(dbFilePath, pw, true)
}

func (a *adb) createBuckets() error {
	return a.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(keysBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(accountsBucket)
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

func (a *adbtx) withTx(dbTx *bolt.Tx, myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if dbTx != nil {
		return myFunc(dbTx)
	} else if writeTx {
		return a.adb.db.Update(myFunc)
	} else {
		return a.adb.db.View(myFunc)
	}
}
