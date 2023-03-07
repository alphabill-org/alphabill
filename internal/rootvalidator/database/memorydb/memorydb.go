package memorydb

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/database"
)

var (
	ErrInvalidKey = errors.New("invalid key")
	ErrValueIsNil = errors.New("value is nil")
)

type (
	EncodeFn func(v any) ([]byte, error)
	DecodeFn func(data []byte, v any) error

	MemoryDB struct {
		db      map[string][]byte
		encoder EncodeFn
		decoder DecodeFn
		lock    sync.RWMutex
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

// New creates a new mock key value db that currently uses map as storage.
// NB! map is probably not the best solution and should be replaced with binary search tree
func New() *MemoryDB {
	return &MemoryDB{
		db:      make(map[string][]byte),
		encoder: json.Marshal,
		decoder: json.Unmarshal,
	}
}

// Empty returns true if no values are stored in db
func (db *MemoryDB) Empty() bool {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if len(db.db) == 0 {
		return true
	}
	return false
}

// Read retrieves the given key if it's present in the key-value store.
func (db *MemoryDB) Read(key []byte, value any) (bool, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if err := checkKeyAndValue(key, value); err != nil {
		return false, err
	}
	if data, ok := db.db[string(key)]; ok {
		return true, db.decoder(data, value)
	}
	return false, nil
}

// Write inserts the given value into the key-value store.
func (db *MemoryDB) Write(key []byte, value any) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	if err := checkKeyAndValue(key, value); err != nil {
		return err
	}
	b, err := db.encoder(value)
	if err != nil {
		return err
	}
	db.db[string(key)] = b
	return nil
}

// Delete removes the key from the key-value store.
func (db *MemoryDB) Delete(key []byte) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	if err := checkKey(key); err != nil {
		return err
	}
	delete(db.db, string(key))
	return nil
}

// First returns forward iterator to the first element in DB
func (db *MemoryDB) First() database.Iterator {
	db.lock.RLock()
	defer db.lock.RUnlock()
	it := NewIterator(db.db, db.decoder)
	it.first()
	return it
}

// Last returns reverse iterator from the last element in DB
func (db *MemoryDB) Last() database.ReverseIterator {
	db.lock.RLock()
	defer db.lock.RUnlock()
	it := NewIterator(db.db, db.decoder)
	it.last()
	return it
}

// Find returns the closest binary search match
func (db *MemoryDB) Find(key []byte) database.Iterator {
	db.lock.RLock()
	defer db.lock.RUnlock()
	it := NewIterator(db.db, db.decoder)
	it.seek(key)
	return it
}

func (db *MemoryDB) StartTx() (database.DBTransaction, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()
	tx, err := NewMapTx(db, db.encoder)
	if err != nil {
		return nil, fmt.Errorf("failed to start Bolt tx, %w", err)
	}
	return tx, nil
}
