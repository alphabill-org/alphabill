package memorydb

import "fmt"

type writeTx struct {
	key    string
	value  []byte
	delete bool
}

type Tx struct {
	db  map[string][]byte
	txs []*writeTx
	enc EncodeFn
}

func NewMapTx(db map[string][]byte, e EncodeFn) (*Tx, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	return &Tx{
		db:  db,
		txs: []*writeTx{},
		enc: e,
	}, nil
}

func (t *Tx) Write(key []byte, value any) error {
	b, err := t.enc(value)
	if err != nil {
		return err
	}
	t.txs = append(t.txs, &writeTx{key: string(key), value: b})
	return nil
}

func (t *Tx) Delete(key []byte) error {
	t.txs = append(t.txs, &writeTx{key: string(key), delete: true})
	return nil
}

func (t *Tx) Rollback() error {
	return nil
}

func (t *Tx) Commit() error {
	for _, tx := range t.txs {
		if tx.delete == true {
			delete(t.db, tx.key)
		} else {
			t.db[tx.key] = tx.value
		}
	}
	return nil
}
