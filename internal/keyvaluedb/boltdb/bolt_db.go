package boltdb

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	bolt "go.etcd.io/bbolt"
)

// bucket feature currently not used as it is not compatible with most others key-value database implementations
// use more than one db file instead
const defaultBucket = "default"

type (
	EncodeFn func(v any) ([]byte, error)
	DecodeFn func(data []byte, v any) error

	BoltDB struct {
		db      *bolt.DB
		bucket  []byte
		encoder EncodeFn
		decoder DecodeFn
	}
)

// New creates a new Bolt DB
// todo: add options and make it possible to use other encode/decode methods
func New(dbFile string) (*BoltDB, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &BoltDB{
		db:      db,
		bucket:  []byte(defaultBucket),
		encoder: json.Marshal,
		decoder: json.Unmarshal,
	}
	if err = s.createBuckets(); err != nil {
		return nil, err
	}
	return s, err
}

func (db *BoltDB) Path() string {
	return db.db.Path()
}

func (db *BoltDB) createBuckets() error {
	return db.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(db.bucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func (db *BoltDB) Empty() bool {
	it := db.First()
	defer it.Close()
	return !it.Valid()
}

func (db *BoltDB) Read(key []byte, v any) (bool, error) {
	if err := keyvaluedb.CheckKeyAndValue(key, v); err != nil {
		return false, err
	}
	var data []byte = nil
	if err := db.db.View(func(tx *bolt.Tx) error {
		data = tx.Bucket(db.bucket).Get(key)
		return nil
	}); err != nil {
		return false, fmt.Errorf("bolt db read failed, %w", err)
	}
	// nil is returned if no value is stored under key
	if data == nil {
		return false, nil
	}
	return true, db.decoder(data, v)
}

func (db *BoltDB) Write(key []byte, v any) error {
	if err := keyvaluedb.CheckKeyAndValue(key, v); err != nil {
		return err
	}
	b, err := db.encoder(v)
	if err != nil {
		return err
	}
	if err = db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(db.bucket).Put(key, b)
	}); err != nil {
		return fmt.Errorf("bolt db write failed, %w", err)
	}
	return nil
}

func (db *BoltDB) Delete(key []byte) error {
	if err := keyvaluedb.CheckKey(key); err != nil {
		return err
	}
	if err := db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(db.bucket).Delete(key)
	}); err != nil {
		return fmt.Errorf("bolt db delete failed, %w", err)
	}
	return nil
}

func (db *BoltDB) First() keyvaluedb.Iterator {
	it := NewIterator(db.db, db.bucket, db.decoder)
	it.first()
	return it
}

func (db *BoltDB) Last() keyvaluedb.ReverseIterator {
	it := NewIterator(db.db, db.bucket, db.decoder)
	it.last()
	return it
}

func (db *BoltDB) Find(key []byte) keyvaluedb.Iterator {
	it := NewIterator(db.db, db.bucket, db.decoder)
	it.seek(key)
	return it
}

func (db *BoltDB) StartTx() (keyvaluedb.DBTransaction, error) {
	tx, err := NewBoltTx(db.db, db.bucket, db.encoder)
	if err != nil {
		return nil, fmt.Errorf("failed to start Bolt tx, %w", err)
	}
	return tx, nil
}
