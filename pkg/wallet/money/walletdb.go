package money

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/alphabill-org/alphabill/internal/crypto"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	bolt "go.etcd.io/bbolt"
)

var (
	keysBucket          = []byte("keys")
	accountsBucket      = []byte("accounts")
	accountBillsBucket  = []byte("accountBills")
	accountDcMetaBucket = []byte("accountDcMeta")
	metaBucket          = []byte("meta")
)

var (
	masterKeyName          = []byte("masterKey")
	mnemonicKeyName        = []byte("mnemonicKey")
	accountKeyName         = []byte("accountKey")
	blockHeightKeyName     = []byte("blockHeightKey")
	isEncryptedKeyName     = []byte("isEncryptedKey")
	maxAccountIndexKeyName = []byte("maxAccountIndexKey")
)

var (
	errWalletDbAlreadyExists = errors.New("wallet db already exists")
	errWalletDbDoesNotExists = errors.New("cannot open wallet db, file does not exist")
	errAccountNotFound       = errors.New("account does not exist")
	errBillNotFound          = errors.New("bill does not exist")
)

const WalletFileName = "wallet.db"

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

	GetBlockNumber() (uint64, error)
	SetBlockNumber(blockNumber uint64) error

	GetBill(accountIndex uint64, id []byte) (*Bill, error)
	SetBill(accountIndex uint64, bill *Bill) error
	ContainsBill(accountIndex uint64, id *uint256.Int) (bool, error)
	RemoveBill(accountIndex uint64, id *uint256.Int) error
	GetBills(accountIndex uint64) ([]*Bill, error)
	GetAllBills() ([][]*Bill, error)
	GetBalance(cmd GetBalanceCmd) (uint64, error)
	GetBalances(cmd GetBalanceCmd) ([]uint64, error)

	GetDcMetadataMap(accountIndex uint64) (map[uint256.Int]*dcMetadata, error)
	GetDcMetadata(accountIndex uint64, nonce []byte) (*dcMetadata, error)
	SetDcMetadata(accountIndex uint64, nonce []byte, dcMetadata *dcMetadata) error
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
	dbFilePath := path.Join(walletDir, WalletFileName)
	return openDb(dbFilePath, config.WalletPass, false)
}

func (w *wdbtx) AddAccount(accountIndex uint64, key *wallet.AccountKey) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(key)
		if err != nil {
			return err
		}
		val, err = w.encryptValue(val)
		if err != nil {
			return err
		}
		accBucket, err := tx.Bucket(accountsBucket).CreateBucketIfNotExists(util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		_, err = accBucket.CreateBucketIfNotExists(accountBillsBucket)
		if err != nil {
			return err
		}
		_, err = accBucket.CreateBucketIfNotExists(accountDcMetaBucket)
		if err != nil {
			return err
		}
		return accBucket.Put(accountKeyName, val)
	}, true)
}

func (w *wdbtx) GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error) {
	var key *wallet.AccountKey
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		k := bkt.Get(accountKeyName)
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

func (w *wdbtx) GetAccountKeys() ([]*wallet.AccountKey, error) {
	keys := make(map[uint64]*wallet.AccountKey)
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(accountsBucket).ForEach(func(accountIndex, v []byte) error {
			if v != nil { // v is nil if entry is a bucket (ignore accounts metadata)
				return nil
			}
			accountBucket, err := getAccountBucket(tx, accountIndex)
			if err != nil {
				return err
			}
			accountKey := accountBucket.Get(accountKeyName)
			accountKeyDecrypted, err := w.decryptValue(accountKey)
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

func (w *wdbtx) SetMaxAccountIndex(accountIndex uint64) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(accountsBucket).Put(maxAccountIndexKeyName, util.Uint64ToBytes(accountIndex))
	}, true)
}

func (w *wdbtx) GetMaxAccountIndex() (uint64, error) {
	var res uint64
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		accountIndex := tx.Bucket(accountsBucket).Get(maxAccountIndexKeyName)
		res = util.BytesToUint64(accountIndex)
		return nil
	}, false)
	if err != nil {
		return 0, err
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
		_, err = w.GetAccountKey(0)
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

func (w *wdbtx) GetBill(accountIndex uint64, billId []byte) (*Bill, error) {
	var b *Bill
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		billBytes := bkt.Bucket(accountBillsBucket).Get(billId)
		if billBytes == nil {
			return errBillNotFound
		}
		b, err = parseBill(billBytes)
		if err != nil {
			return err
		}
		return nil
	}, false)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (w *wdbtx) SetBill(accountIndex uint64, bill *Bill) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("adding bill: value=%d id=%s, for account=%d", bill.Value, bill.Id.String(), accountIndex))
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		return bkt.Bucket(accountBillsBucket).Put(bill.GetID(), val)
	}, true)
}

func (w *wdbtx) ContainsBill(accountIndex uint64, id *uint256.Int) (bool, error) {
	var res bool
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		billId := id.Bytes32()
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		res = bkt.Bucket(accountBillsBucket).Get(billId[:]) != nil
		return nil
	}, false)
	if err != nil {
		return false, err
	}
	return res, nil
}

func (w *wdbtx) GetBills(accountIndex uint64) ([]*Bill, error) {
	var res []*Bill
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		c := bkt.Bucket(accountBillsBucket).Cursor()
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

func (w *wdbtx) GetAllBills() ([][]*Bill, error) {
	var res [][]*Bill
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		maxAccountIndex, err := w.GetMaxAccountIndex()
		if err != nil {
			return err
		}
		for accountIndex := uint64(0); accountIndex <= maxAccountIndex; accountIndex++ {
			accountBills, err := w.GetBills(accountIndex)
			if err != nil {
				return err
			}
			res = append(res, accountBills)
		}
		return nil
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (w *wdbtx) RemoveBill(accountIndex uint64, id *uint256.Int) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		bytes32 := id.Bytes32()
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		return bkt.Bucket(accountBillsBucket).Delete(bytes32[:])
	}, true)
}

func (w *wdbtx) GetBalance(cmd GetBalanceCmd) (uint64, error) {
	sum := uint64(0)
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		bkt, err := getAccountBucket(tx, util.Uint64ToBytes(cmd.AccountIndex))
		if err != nil {
			return err
		}
		return bkt.Bucket(accountBillsBucket).ForEach(func(k, v []byte) error {
			var b *Bill
			err := json.Unmarshal(v, &b)
			if err != nil {
				return err
			}
			if b.IsDcBill && !cmd.CountDCBills {
				return nil
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

func (w *wdbtx) GetBalances(cmd GetBalanceCmd) ([]uint64, error) {
	res := make(map[uint64]uint64)
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(accountsBucket).ForEach(func(accIdx, v []byte) error {
			if v != nil { // value is nil if entry is a bucket
				return nil
			}
			sum := uint64(0)
			accountBucket, err := getAccountBucket(tx, accIdx)
			if err != nil {
				return err
			}
			accBillsBucket := accountBucket.Bucket(accountBillsBucket)
			err = accBillsBucket.ForEach(func(billId, billValue []byte) error {
				var b *Bill
				err := json.Unmarshal(billValue, &b)
				if err != nil {
					return err
				}
				if b.IsDcBill && !cmd.CountDCBills {
					return nil
				}
				sum += b.Value
				return nil
			})
			if err != nil {
				return err
			}
			res[util.BytesToUint64(accIdx)] = sum
			return nil
		})
	}, false)
	if err != nil {
		return nil, err
	}
	balances := make([]uint64, len(res))
	for accIdx, sum := range res {
		balances[accIdx] = sum
	}
	return balances, nil
}

func (w *wdbtx) GetBlockNumber() (uint64, error) {
	var res uint64
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		blockHeightBytes := tx.Bucket(metaBucket).Get(blockHeightKeyName)
		if blockHeightBytes == nil {
			return nil
		}
		res = util.BytesToUint64(blockHeightBytes)
		return nil
	}, false)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (w *wdbtx) SetBlockNumber(blockHeight uint64) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(blockHeightKeyName, util.Uint64ToBytes(blockHeight))
	}, true)
}

func (w *wdbtx) GetDcMetadataMap(accountIndex uint64) (map[uint256.Int]*dcMetadata, error) {
	res := map[uint256.Int]*dcMetadata{}
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		accountBucket, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		return accountBucket.Bucket(accountDcMetaBucket).ForEach(func(k, v []byte) error {
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

func (w *wdbtx) GetDcMetadata(accountIndex uint64, nonce []byte) (*dcMetadata, error) {
	var res *dcMetadata
	err := w.withTx(w.tx, func(tx *bolt.Tx) error {
		accountBucket, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		m := accountBucket.Bucket(accountDcMetaBucket).Get(nonce)
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

func (w *wdbtx) SetDcMetadata(accountIndex uint64, dcNonce []byte, dcMetadata *dcMetadata) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		accountBucket, err := getAccountBucket(tx, util.Uint64ToBytes(accountIndex))
		if err != nil {
			return err
		}
		if dcMetadata != nil {
			val, err := json.Marshal(dcMetadata)
			if err != nil {
				return err
			}
			return accountBucket.Bucket(accountDcMetaBucket).Put(dcNonce, val)
		}
		return accountBucket.Bucket(accountDcMetaBucket).Delete(dcNonce)
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
		_, err = tx.CreateBucketIfNotExists(accountsBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(accountDcMetaBucket)
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

	dbFilePath := path.Join(walletDir, WalletFileName)
	return openDb(dbFilePath, config.WalletPass, true)
}

func parseBill(v []byte) (*Bill, error) {
	var b *Bill
	err := json.Unmarshal(v, &b)
	return b, err
}

func getAccountBucket(tx *bolt.Tx, accountIndex []byte) (*bolt.Bucket, error) {
	bkt := tx.Bucket(accountsBucket).Bucket(accountIndex)
	if bkt == nil {
		return nil, errAccountNotFound
	}
	return bkt, nil
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
