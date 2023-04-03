package memorydb

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
)

type (
	EncodeFn func(v any) ([]byte, error)
	DecodeFn func(data []byte, v any) error

	MemoryDB struct {
		db      map[string][]byte
		encoder EncodeFn
		decoder DecodeFn
		limit   int
		lock    sync.RWMutex
	}
)

// New creates a new mock key value db that currently uses map as storage.
// NB! map is probably not the best solution and should be replaced with binary search tree
func New() *MemoryDB {
	return &MemoryDB{
		db:      make(map[string][]byte),
		encoder: json.Marshal,
		decoder: json.Unmarshal,
	}
}

// NewWithLimiter can be used to test disk full scenarios
func NewWithLimiter(limit int) *MemoryDB {
	return &MemoryDB{
		db:      make(map[string][]byte),
		encoder: json.Marshal,
		decoder: json.Unmarshal,
		limit:   limit,
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

	if err := keyvaluedb.CheckKeyAndValue(key, value); err != nil {
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
	if err := keyvaluedb.CheckKeyAndValue(key, value); err != nil {
		return err
	}
	b, err := db.encoder(value)
	if err != nil {
		return err
	}
	if db.limit > 0 && len(db.db) >= db.limit {
		return fmt.Errorf("write failed, disk is full")
	}
	db.db[string(key)] = b
	return nil
}

// Delete removes the key from the key-value store.
func (db *MemoryDB) Delete(key []byte) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	if err := keyvaluedb.CheckKey(key); err != nil {
		return err
	}
	delete(db.db, string(key))
	return nil
}

// First returns forward iterator to the first element in DB
func (db *MemoryDB) First() keyvaluedb.Iterator {
	db.lock.RLock()
	defer db.lock.RUnlock()
	it := NewIterator(db.db, db.decoder)
	it.first()
	return it
}

// Last returns reverse iterator from the last element in DB
func (db *MemoryDB) Last() keyvaluedb.ReverseIterator {
	db.lock.RLock()
	defer db.lock.RUnlock()
	it := NewIterator(db.db, db.decoder)
	it.last()
	return it
}

// Find returns the closest binary search match
func (db *MemoryDB) Find(key []byte) keyvaluedb.Iterator {
	db.lock.RLock()
	defer db.lock.RUnlock()
	it := NewIterator(db.db, db.decoder)
	it.seek(key)
	return it
}

func (db *MemoryDB) StartTx() (keyvaluedb.DBTransaction, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()
	tx, err := NewMapTx(db)
	if err != nil {
		return nil, fmt.Errorf("failed to start Bolt tx, %w", err)
	}
	return tx, nil
}

func (db *MemoryDB) SetLimit(limit int) {
	db.lock.RLock()
	defer db.lock.RUnlock()
	db.limit = limit
}
