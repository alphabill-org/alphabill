package tokens

import (
	"encoding/json"
	"errors"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	bolt "go.etcd.io/bbolt"
	"os"
	"path"
)

var (
	accountBillsBucket = []byte("accountTokens")
	metaBucket         = []byte("meta")
)

var (
	blockHeightKeyName = []byte("blockHeightKey")
)

var (
	errWalletDbAlreadyExists = errors.New("wallet db already exists")
	errWalletDbDoesNotExists = errors.New("cannot open tokens db, file does not exist")
	errAccountNotFound       = errors.New("account does not exist")
)

const tokensFileName = "tokens.db"

type (
	tokenId []byte

	token struct {
		Id   tokenId   `json:"id"`
		Kind TokenKind `json:"kind"`
	}
)

type Db interface {
	Do() TokenTxContext
	WithTransaction(func(tx TokenTxContext) error) error
	Close()
	DeleteDb()
}

type TokenTxContext interface {
	GetBlockNumber() (uint64, error)
	SetBlockNumber(blockNumber uint64) error

	SetToken(accountIndex uint64, token *token) error
	ContainsToken(accountIndex uint64, id tokenId) (bool, error)
	RemoveToken(accountIndex uint64, id tokenId) error
	GetTokens(accountIndex uint64) ([]*token, error)
}

type tokensDb struct {
	db         *bolt.DB
	dbFilePath string
}

type tokensDbTx struct {
	db *tokensDb
	tx *bolt.Tx
}

func (t *tokensDbTx) SetToken(accountIndex uint64, token *token) error {
	//TODO implement me
	panic("implement me")
}

func (t *tokensDbTx) ContainsToken(accountIndex uint64, id tokenId) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (t *tokensDbTx) RemoveToken(accountIndex uint64, id tokenId) error {
	//TODO implement me
	panic("implement me")
}

func (t *tokensDbTx) GetTokens(accountIndex uint64) ([]*token, error) {
	//TODO implement me
	panic("implement me")
}

func (w *tokensDbTx) GetBlockNumber() (uint64, error) {
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

func (w *tokensDbTx) SetBlockNumber(blockHeight uint64) error {
	return w.withTx(w.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(blockHeightKeyName, util.Uint64ToBytes(blockHeight))
	}, true)
}

func (w *tokensDb) DeleteDb() {
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

func (w *tokensDb) WithTransaction(fn func(txc TokenTxContext) error) error {
	return w.db.Update(func(tx *bolt.Tx) error {
		return fn(&tokensDbTx{db: w, tx: tx})
	})
}

func (w *tokensDb) Do() TokenTxContext {
	return &tokensDbTx{db: w, tx: nil}
}

func (w *tokensDb) Path() string {
	return w.dbFilePath
}

func (w *tokensDb) Close() {
	if w.db == nil {
		return
	}
	log.Info("closing wallet db")
	err := w.db.Close()
	if err != nil {
		log.Warning("error closing db: ", err)
	}
}

func (w *tokensDbTx) withTx(dbTx *bolt.Tx, myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if dbTx != nil {
		return myFunc(dbTx)
	} else if writeTx {
		return w.db.db.Update(myFunc)
	} else {
		return w.db.db.View(myFunc)
	}
}

func (w *tokensDb) createBuckets() error {
	return w.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func openTokensDb(walletDir string) (*tokensDb, error) {
	err := os.MkdirAll(walletDir, 0700) // -rwx------
	if err != nil {
		return nil, err
	}
	dbFilePath := path.Join(walletDir, tokensFileName)

	db, err := bolt.Open(dbFilePath, 0600, nil) // -rw-------
	if err != nil {
		return nil, err
	}

	w := &tokensDb{db, dbFilePath}
	err = w.createBuckets()
	if err != nil {
		return nil, err
	}
	return w, nil
}

func parseToken(v []byte) (*token, error) {
	var t *token
	err := json.Unmarshal(v, &t)
	return t, err
}
