package boltdb

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	bolt "go.etcd.io/bbolt"
)

type Tx struct {
	tx  *bolt.Tx
	b   *bolt.Bucket
	enc EncodeFn
}

func NewBoltTx(db *bolt.DB, bucket []byte, e EncodeFn) (*Tx, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}

	return &Tx{
		tx:  tx,
		b:   tx.Bucket(bucket),
		enc: e,
	}, nil
}

func (t *Tx) Write(key []byte, value any) error {
	if err := keyvaluedb.CheckKeyAndValue(key, value); err != nil {
		return err
	}
	b, err := t.enc(value)
	if err != nil {
		return err
	}
	return t.b.Put(key, b)
}

func (t *Tx) Delete(key []byte) error {
	if err := keyvaluedb.CheckKey(key); err != nil {
		return err
	}
	return t.b.Delete(key)
}

func (t *Tx) Rollback() error {
	return t.tx.Rollback()
}

func (t *Tx) Commit() error {
	return t.tx.Commit()
}
