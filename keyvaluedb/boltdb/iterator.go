package boltdb

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

type (
	Itr struct {
		tx      *bolt.Tx
		cursor  *bolt.Cursor
		decoder DecodeFn
		key     []byte
		value   []byte
	}
)

func (it *Itr) Close() error {
	if it.tx == nil {
		return nil
	}
	// cursor seems error is only returned if already closed
	err := it.tx.Rollback()
	// release iterator, so cursor cannot be closed twice - hence this should never return error
	it.tx = nil
	it.key = nil
	it.value = nil
	return err
}

func NewIterator(db *bolt.DB, bucket []byte, d DecodeFn) *Itr {
	it, err := newIterator(db, bucket, d)
	if err != nil {
		return &Itr{}
	}
	return it
}

func (it *Itr) Next() {
	if !it.Valid() {
		return
	}
	it.key, it.value = it.cursor.Next()
}
func (it *Itr) Prev() {
	if !it.Valid() {
		return
	}
	it.key, it.value = it.cursor.Prev()
}

func (it *Itr) Valid() bool {
	return it.key != nil
}

func (it *Itr) Key() []byte {
	return it.key
}

func (it *Itr) Value(v any) error {
	if !it.Valid() {
		return fmt.Errorf("iterator invalid")
	}
	return it.decoder(it.value, v)
}

func newIterator(db *bolt.DB, bucket []byte, d DecodeFn) (*Itr, error) {
	if db == nil {
		return nil, fmt.Errorf("bolt db is nil")
	}
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	b := tx.Bucket(bucket)
	return &Itr{
		tx:      tx,
		cursor:  b.Cursor(),
		decoder: d,
	}, nil
}

func (it *Itr) first() {
	if it.tx != nil {
		it.key, it.value = it.cursor.First()
	}
}

func (it *Itr) last() {
	if it.tx != nil {
		it.key, it.value = it.cursor.Last()
	}
}

func (it *Itr) seek(key []byte) {
	if it.tx != nil {
		it.key, it.value = it.cursor.Seek(key)
	}
}
