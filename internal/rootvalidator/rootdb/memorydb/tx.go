package memorydb

import "fmt"

type writeTx struct {
	key    string
	value  []byte
	delete bool
}

type Tx struct {
	mem *MemoryDB
	txs []*writeTx
	enc EncodeFn
}

func NewMapTx(m *MemoryDB, e EncodeFn) (*Tx, error) {
	if m == nil {
		return nil, fmt.Errorf("momory db is nil")
	}
	if m.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	return &Tx{
		mem: m,
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
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	for _, tx := range t.txs {
		if tx.delete == true {
			delete(t.mem.db, tx.key)
		} else {
			t.mem.db[tx.key] = tx.value
		}
	}
	return nil
}
