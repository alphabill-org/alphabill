package memorydb

import (
	"fmt"

	"github.com/alphabill-org/alphabill/validator/internal/keyvaluedb"
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
		db:  copyMap(m.db),
	}, nil
}

func (t *Tx) Read(key []byte, v any) (bool, error) {
	t.mem.lock.RLock()
	defer t.mem.lock.RUnlock()
	if err := keyvaluedb.CheckKeyAndValue(key, v); err != nil {
		return false, err
	}
	if t.db == nil {
		return false, fmt.Errorf("memdb tx read failed, tx closed")
	}
	if data, ok := t.db[string(key)]; ok {
		return true, t.mem.decoder(data, v)
	}
	return false, nil
}

func (t *Tx) Write(key []byte, value any) error {
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	if err := keyvaluedb.CheckKeyAndValue(key, value); err != nil {
		return err
	}
	if t.db == nil {
		return fmt.Errorf("memdb tx write failed, tx closed")
	}
	b, err := t.mem.encoder(value)
	if err != nil {
		return err
	}
	if t.mem.writeErr != nil {
		return t.mem.writeErr
	}
	t.db[string(key)] = b
	return nil
}

func (t *Tx) Delete(key []byte) error {
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	if err := keyvaluedb.CheckKey(key); err != nil {
		return err
	}
	if t.db == nil {
		return fmt.Errorf("memdb tx delete failed, tx closed")
	}
	delete(t.db, string(key))
	return nil
}

func (t *Tx) Rollback() error {
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	t.db = nil
	return nil
}

func (t *Tx) Commit() error {
	t.mem.lock.Lock()
	defer t.mem.lock.Unlock()
	t.mem.db = t.db
	t.db = nil
	return nil
}
