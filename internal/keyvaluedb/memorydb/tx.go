package memorydb

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
)

type Tx struct {
	mem *MemoryDB
	db  map[string][]byte
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	result := make(map[K]V, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func NewMapTx(m *MemoryDB) (*Tx, error) {
	if m == nil {
		return nil, fmt.Errorf("momory db is nil")
	}
	if m.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	return &Tx{
		mem: m,
		db:  nil,
	}, nil
}

func (t *Tx) copyOnWrite() {
	t.db = copyMap(t.mem.db)
}

func (t *Tx) Write(key []byte, value any) error {
	if err := keyvaluedb.CheckKeyAndValue(key, value); err != nil {
		return err
	}
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	b, err := t.mem.encoder(value)
	if err != nil {
		return err
	}
	// copy on write
	if t.db == nil {
		t.copyOnWrite()
	}
	if t.mem.writeErr != nil {
		return t.mem.writeErr
	}
	t.db[string(key)] = b
	return nil
}

func (t *Tx) Delete(key []byte) error {
	if err := keyvaluedb.CheckKey(key); err != nil {
		return err
	}
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	// copy on write
	if t.db == nil {
		t.copyOnWrite()
	}
	delete(t.db, string(key))
	return nil
}

func (t *Tx) Rollback() error {
	return nil
}

func (t *Tx) Commit() error {
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	t.mem.db = t.db
	t.db = nil
	return nil
}
