package tokens

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	bolt "go.etcd.io/bbolt"
)

var (
	accountsBucket      = []byte("accounts")
	accountTokensBucket = []byte("accountTokens")
	tokenTypes          = []byte("tokenTypes")
	metaBucket          = []byte("meta")

	blockHeightKeyName = []byte("blockHeightKey")
)

const (
	tokensFileName = "tokens.db"
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

	AddTokenType(token *TokenUnitType) error
	GetTokenType(typeId TokenTypeID) (*TokenUnitType, error)
	GetTokenTypes() ([]*TokenUnitType, error)
	// SetToken accountNumber == 0 is the one for "always true" predicates
	// keys with accountIndex from the money wallet have tokens here under accountNumber which is accountIndex+1
	SetToken(accountNumber uint64, token *TokenUnit) error
	RemoveToken(accountNumber uint64, id TokenID) error
	GetToken(accountNumber uint64, tokenId TokenID) (*TokenUnit, error)
	GetTokens(accountNumber uint64) ([]*TokenUnit, error)
}

type tokensDb struct {
	db         *bolt.DB
	dbFilePath string
}

type tokensDbTx struct {
	db *tokensDb
	tx *bolt.Tx
}

func (t *tokensDbTx) AddTokenType(tType *TokenUnitType) error {
	return t.withTx(t.tx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(tType)
		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("adding token type: id=%X, symbol=%s", tType.ID, tType.Symbol))
		return tx.Bucket(tokenTypes).Put(tType.ID, val)
	}, true)
}

// GetTokenType returns empty structure if typeId not found
func (t *tokensDbTx) GetTokenType(typeId TokenTypeID) (*TokenUnitType, error) {
	var tokenType *TokenUnitType
	err := t.withTx(t.tx, func(tx *bolt.Tx) error {
		res, err := parseTokenType(tx.Bucket(tokenTypes).Get(typeId))
		if err != nil {
			return err
		}
		tokenType = res
		return nil
	}, false)

	if err != nil {
		return nil, err
	}
	return tokenType, nil
}

func (t *tokensDbTx) GetTokenTypes() ([]*TokenUnitType, error) {
	var types []*TokenUnitType
	err := t.withTx(t.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(tokenTypes).ForEach(func(k, v []byte) error {
			t, err := parseTokenType(v)
			if err != nil {
				return err
			}
			types = append(types, t)
			return nil
		})
	}, false)

	if err != nil {
		return nil, err
	}
	return types, nil
}

func (t *tokensDbTx) SetToken(accountNumber uint64, token *TokenUnit) error {
	return t.withTx(t.tx, func(tx *bolt.Tx) error {
		val, err := json.Marshal(token)
		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("adding token: id=%X, for account=%d, bl=%X", token.ID, accountNumber, token.Backlink))
		bkt, err := ensureTokenBucket(tx, util.Uint64ToBytes(accountNumber))
		if err != nil {
			return err
		}
		return bkt.Put(token.ID, val)
	}, true)
}

func ensureTokenBucket(tx *bolt.Tx, accountNumber []byte) (*bolt.Bucket, error) {
	b, err := tx.CreateBucketIfNotExists(accountsBucket)
	if err != nil {
		return nil, err
	}
	b, err = b.CreateBucketIfNotExists(accountNumber)
	if err != nil {
		return nil, err
	}
	b, err = b.CreateBucketIfNotExists(accountTokensBucket)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (t *tokensDbTx) RemoveToken(accountNumber uint64, id TokenID) error {
	return t.withTx(t.tx, func(tx *bolt.Tx) error {
		log.Info(fmt.Sprintf("removing token: id=%X, for account=%d", id, accountNumber))
		bkt, err := ensureTokenBucket(tx, util.Uint64ToBytes(accountNumber))
		if err != nil {
			return err
		}
		return bkt.Delete(id)
	}, true)
}

func (t *tokensDbTx) GetToken(accountNumber uint64, tokenId TokenID) (*TokenUnit, error) {
	var tok *TokenUnit
	err := t.withTx(t.tx, func(tx *bolt.Tx) error {
		bkt, err := ensureTokenBucket(tx, util.Uint64ToBytes(accountNumber))
		if err != nil {
			return err
		}
		raw := bkt.Get(tokenId)
		if raw == nil {
			return nil
		}
		res, err := parseToken(raw)
		if err != nil {
			return err
		}
		tok = res
		return nil
	}, true)

	if err != nil {
		return nil, err
	}
	return tok, nil
}

func (t *tokensDbTx) GetTokens(accountNumber uint64) ([]*TokenUnit, error) {
	var tokens []*TokenUnit
	err := t.withTx(t.tx, func(tx *bolt.Tx) error {
		bkt, err := ensureTokenBucket(tx, util.Uint64ToBytes(accountNumber))
		if err != nil {
			return err
		}
		return bkt.ForEach(func(k, v []byte) error {
			t, err := parseToken(v)
			if err != nil {
				return err
			}
			tokens = append(tokens, t)
			return nil
		})
	}, true)
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

func (t *tokensDbTx) GetBlockNumber() (uint64, error) {
	var res uint64
	err := t.withTx(t.tx, func(tx *bolt.Tx) error {
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

func (t *tokensDbTx) SetBlockNumber(blockHeight uint64) error {
	return t.withTx(t.tx, func(tx *bolt.Tx) error {
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

func (t *tokensDbTx) withTx(dbTx *bolt.Tx, myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if dbTx != nil {
		return myFunc(dbTx)
	} else if writeTx {
		return t.db.db.Update(myFunc)
	} else {
		return t.db.db.View(myFunc)
	}
}

func (w *tokensDb) createBuckets() error {
	return w.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(accountsBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(tokenTypes)
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

func parseTokenType(v []byte) (*TokenUnitType, error) {
	var t = &TokenUnitType{}
	if v == nil {
		return t, nil
	}

	err := json.Unmarshal(v, &t)
	return t, err
}

func parseToken(v []byte) (*TokenUnit, error) {
	var t *TokenUnit
	err := json.Unmarshal(v, &t)
	return t, err
}
