package boltdb

import (
	"fmt"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	bolt "go.etcd.io/bbolt"
)

type Tx struct {
	tx  *bolt.Tx
	b   *bolt.Bucket
	enc EncodeFn
	dec DecodeFn
}

func NewBoltTx(db *bolt.DB, bucket []byte, e EncodeFn, d DecodeFn) (*Tx, error) {
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
		dec: d,
	}, nil
}

func (t *Tx) Read(key []byte, value any) (bool, error) {
	if t.tx.DB() == nil {
		return false, fmt.Errorf("bolt tx read failed, %w", bolt.ErrTxClosed)
	}
	if err := keyvaluedb.CheckKeyAndValue(key, value); err != nil {
		return false, err
	}
	b := t.b.Get(key)
	if b == nil {
		return false, nil
	}
	return true, t.dec(b, value)
}

func (t *Tx) Write(key []byte, value any) error {
	if t.tx.DB() == nil {
		return fmt.Errorf("bolt tx write failed, %w", bolt.ErrTxClosed)
	}
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
	if t.tx.DB() == nil {
		return fmt.Errorf("bolt tx delete failed, %w", bolt.ErrTxClosed)
	}
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
