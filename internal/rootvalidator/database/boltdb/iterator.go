package boltdb

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

type (
	Itr struct {
		tx      *bolt.Tx
		it      *bolt.Cursor
		decoder DecodeFn
		key     []byte
		value   []byte
	}
)

func (it *Itr) Close() error {
	if it.tx != nil {
		return it.tx.Rollback()
	} else {
		return nil
	}
}

func NewIterator(db *bolt.DB, bucket []byte, d DecodeFn) *Itr {
	it, err := newIterator(db, bucket, d)
	if err != nil {
		return &Itr{}
	}
	return it
}

func (it *Itr) Next() {
	it.key, it.value = it.it.Next()
}
func (it *Itr) Prev() {
	it.key, it.value = it.it.Prev()
}

func (it *Itr) Valid() bool {
	return !(it.key == nil)
}

func (it *Itr) Key() []byte {
	return it.key
}
func (it *Itr) Value(v any) error {
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
		it:      b.Cursor(),
		decoder: d,
	}, nil
}

func (it *Itr) first() {
	if it.tx != nil {
		it.key, it.value = it.it.First()
	}
}

func (it *Itr) last() {
	if it.tx != nil {
		it.key, it.value = it.it.Last()
	}
}

func (it *Itr) seek(key []byte) {
	if it.tx != nil {
		it.key, it.value = it.it.Seek(key)
	}
}