package boltdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/rootdb"
	bolt "go.etcd.io/bbolt"
)

const defaultBucket = "default"

var (
	ErrInvalidKey = errors.New("invalid key")
	ErrValueIsNil = errors.New("value is nil")
)

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

func checkKey(key []byte) error {
	if len(key) == 0 {
		return ErrInvalidKey
	}
	return nil
}

func checkValue(val any) error {
	if reflect.ValueOf(val).Kind() == reflect.Ptr && reflect.ValueOf(val).IsNil() {
		return ErrValueIsNil
	}
	return nil
}

func checkKeyAndValue(key []byte, val any) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if err := checkValue(val); err != nil {
		return err
	}
	return nil
}

func New(dbFile string) (*BoltDB, error) {
	db, err := bolt.Open(dbFile, 0600, nil)
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
	if err := checkKeyAndValue(key, v); err != nil {
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
	if err := checkKeyAndValue(key, v); err != nil {
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
	if err := checkKey(key); err != nil {
		return err
	}
	if err := db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(db.bucket).Delete(key)
	}); err != nil {
		return fmt.Errorf("bolt db delete failed, %w", err)
	}
	return nil
}

func (db *BoltDB) First() rootdb.Iterator {
	it := NewIterator(db.db, db.bucket, db.decoder)
	it.first()
	return it
}

func (db *BoltDB) Last() rootdb.ReverseIterator {
	it := NewIterator(db.db, db.bucket, db.decoder)
	it.last()
	return it
}

func (db *BoltDB) Find(key []byte) rootdb.Iterator {
	it := NewIterator(db.db, db.bucket, db.decoder)
	it.seek(key)
	return it
}

func (db *BoltDB) StartTx() (rootdb.DBTransaction, error) {
	tx, err := NewBoltTx(db.db, db.bucket, db.encoder)
	if err != nil {
		return nil, fmt.Errorf("failed to start Bolt tx, %w", err)
	}
	return tx, nil
}
