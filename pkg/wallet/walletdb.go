package wallet

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	bolt "go.etcd.io/bbolt"
	"os"
	"path"
	"strings"
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
	errWalletDbDoesNotExists    = errors.New("cannot open wallet db, file does not exits")
	errKeyNotFound              = errors.New("key not found in wallet")
	errBillWithMinValueNotFound = errors.New("spendable bill with min value not found")
)

const walletFileName = "wallet.db"

type Db interface {
	GetAccountKey(dbTx any) (*accountKey, error)
	SetAccountKey(dbTx any, key *accountKey) error

	GetMasterKey(dbTx any) (string, error)
	SetMasterKey(dbTx any, masterKey string) error

	GetMnemonic(dbTx any) (string, error)
	SetMnemonic(dbTx any, mnemonic string) error

	IsEncrypted(dbTx any) (bool, error)
	SetEncrypted(dbTx any, encrypted bool) error
	VerifyPassword(dbTx any) (bool, error)

	SetBill(dbTx any, bill *bill) error
	ContainsBill(dbTx any, id *uint256.Int) (bool, error)
	RemoveBill(dbTx any, id *uint256.Int) error
	GetBills(dbTx any) ([]*bill, error)
	GetBillWithMinValue(dbTx any, minVal uint64) (*bill, error)
	GetBalance(dbTx any) (uint64, error)
	GetBlockHeight(dbTx any) (uint64, error)
	SetBlockHeight(dbTx any, blockHeight uint64) error

	GetDcMetadataMap(dbTx any) (map[uint256.Int]*dcMetadata, error)
	GetDcMetadata(dbTx any, nonce []byte) (*dcMetadata, error)
	SetDcMetadata(dbTx any, nonce []byte, dcMetadata *dcMetadata) error

	WithTransaction(func(dbTx any) error) error

	Close()
	DeleteDb()
}

type wdb struct {
	db         *bolt.DB
	dbFilePath string
	walletPass string
}

func OpenDb(config Config) (*wdb, error) {
	walletDir, err := config.GetWalletDir()
	if err != nil {
		return nil, err
	}
	dbFilePath := path.Join(walletDir, walletFileName)
	return openDb(dbFilePath, config.WalletPass, false)
}

func (w *wdb) SetAccountKey(dbTx any, key *accountKey) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		val, err = w.encryptValue(dbTx, val)
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(accountKeyName, val)
	}, true)
}

func (w *wdb) GetAccountKey(dbTx any) (*accountKey, error) {
	var key *accountKey
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
		k := tx.Bucket(keysBucket).Get(accountKeyName)
		if k == nil {
			return errKeyNotFound
		}
		val, err := w.decryptValue(dbTx, k)
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

func (w *wdb) SetMasterKey(dbTx any, masterKey string) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
		val, err := w.encryptValue(dbTx, []byte(masterKey))
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(masterKeyName, val)
	}, true)
}

func (w *wdb) GetMasterKey(dbTx any) (string, error) {
	var res string
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
		masterKey := tx.Bucket(keysBucket).Get(masterKeyName)
		if masterKey == nil {
			return errKeyNotFound
		}
		val, err := w.decryptValue(dbTx, masterKey)
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

func (w *wdb) SetMnemonic(dbTx any, mnemonic string) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
		val, err := w.encryptValue(dbTx, []byte(mnemonic))
		if err != nil {
			return err
		}
		return tx.Bucket(keysBucket).Put(mnemonicKeyName, val)
	}, true)
}

func (w *wdb) GetMnemonic(dbTx any) (string, error) {
	var res string
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
		mnemonic := tx.Bucket(keysBucket).Get(mnemonicKeyName)
		if mnemonic == nil {
			return errKeyNotFound
		}
		val, err := w.decryptValue(dbTx, mnemonic)
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

func (w *wdb) SetEncrypted(dbTx any, encrypted bool) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
		var b byte
		if encrypted {
			b = 0x01
		} else {
			b = 0x00
		}
		return tx.Bucket(metaBucket).Put(isEncryptedKeyName, []byte{b})
	}, true)
}

func (w *wdb) IsEncrypted(dbTx any) (bool, error) {
	var res bool
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
		encrypted := tx.Bucket(metaBucket).Get(isEncryptedKeyName)
		res = bytes.Equal(encrypted, []byte{0x01})
		return nil
	}, false)
	if err != nil {
		return false, err
	}
	return res, nil
}

func (w *wdb) VerifyPassword(dbTx any) (bool, error) {
	encrypted, err := w.IsEncrypted(dbTx)
	if err != nil {
		return false, err
	}
	if encrypted {
		_, err = w.GetAccountKey(dbTx)
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

func (w *wdb) SetBill(dbTx any, bill *bill) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		return tx.Bucket(billsBucket).Put(bill.getId(), val)
	}, true)
}

func (w *wdb) ContainsBill(dbTx any, id *uint256.Int) (bool, error) {
	var res bool
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		res = tx.Bucket(billsBucket).Get(billId[:]) != nil
		return nil
	}, false)
	if err != nil {
		return false, err
	}
	return res, nil
}

func (w *wdb) GetBills(dbTx any) ([]*bill, error) {
	var res []*bill
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
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

func (w *wdb) RemoveBill(dbTx any, id *uint256.Int) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
		bytes32 := id.Bytes32()
		return tx.Bucket(billsBucket).Delete(bytes32[:])
	}, true)
}

func (w *wdb) GetBillWithMinValue(dbTx any, minVal uint64) (*bill, error) {
	var res *bill
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
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

func (w *wdb) GetBalance(dbTx any) (uint64, error) {
	sum := uint64(0)
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
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

func (w *wdb) GetBlockHeight(dbTx any) (uint64, error) {
	var res uint64
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
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

func (w *wdb) SetBlockHeight(dbTx any, blockHeight uint64) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, blockHeight)
		return tx.Bucket(metaBucket).Put(blockHeightKeyName, b)
	}, true)
}

func (w *wdb) GetDcMetadataMap(dbTx any) (map[uint256.Int]*dcMetadata, error) {
	res := map[uint256.Int]*dcMetadata{}
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
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

func (w *wdb) GetDcMetadata(dbTx any, nonce []byte) (*dcMetadata, error) {
	var res *dcMetadata
	err := w.withTx(dbTx, func(tx *bolt.Tx) error {
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

func (w *wdb) SetDcMetadata(dbTx any, dcNonce []byte, dcMetadata *dcMetadata) error {
	return w.withTx(dbTx, func(tx *bolt.Tx) error {
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

func (w *wdb) WithTransaction(myFunc func(dbTx any) error) error {
	return w.db.Update(func(tx *bolt.Tx) error {
		return myFunc(tx)
	})
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

func (w *wdb) withTx(dbTx any, myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if dbTx != nil {
		return myFunc(dbTx.(*bolt.Tx))
	} else if writeTx {
		return w.db.Update(myFunc)
	} else {
		return w.db.View(myFunc)
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

func createNewDb(config Config) (*wdb, error) {
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

func (w *wdb) encryptValue(dbTx any, val []byte) ([]byte, error) {
	isEncrypted, err := w.IsEncrypted(dbTx)
	if err != nil {
		return nil, err
	}
	if !isEncrypted {
		return val, nil
	}
	encryptedValue, err := crypto.Encrypt(w.walletPass, val)
	if err != nil {
		return nil, err
	}
	return []byte(encryptedValue), nil
}

func (w *wdb) decryptValue(dbTx any, val []byte) ([]byte, error) {
	isEncrypted, err := w.IsEncrypted(dbTx)
	if err != nil {
		return nil, err
	}
	if !isEncrypted {
		return val, nil
	}
	decryptedValue, err := crypto.Decrypt(w.walletPass, string(val))
	if err != nil {
		return nil, err
	}
	return decryptedValue, nil
}
