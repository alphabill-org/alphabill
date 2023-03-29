package memorydb

import (
	"bytes"
	"fmt"
	"sort"
)

type (
	Itr struct {
		keys    [][]byte
		values  [][]byte
		decoder DecodeFn
		index   int
	}
)

func (it *Itr) Close() error {
	return nil
}

func NewIterator(db map[string][]byte, d DecodeFn) *Itr {
	it, err := newIterator(db, d)
	if err != nil {
		return &Itr{index: -1}
	}
	return it
}

func (it *Itr) Next() {
	if !it.Valid() {
		return
	}
	it.index++
	if it.index >= len(it.keys) {
		it.index = -1
	}
}
func (it *Itr) Prev() {
	if !it.Valid() {
		return
	}
	it.index--
}

func (it *Itr) Valid() bool {
	return it.index >= 0
}

func (it *Itr) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.keys[it.index]
}
func (it *Itr) Value(v any) error {
	if !it.Valid() {
		return fmt.Errorf("iterator invalid")
	}
	return it.decoder(it.values[it.index], v)
}

func newIterator(db map[string][]byte, d DecodeFn) (*Itr, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	var (
		keys   = make([][]byte, 0, len(db))
		values = make([][]byte, 0, len(db))
	)
	// Collect the keys from the memory database corresponding to the given prefix
	// and start
	for key := range db {
		keys = append(keys, []byte(key))
	}
	// Sort the items and retrieve the associated values
	sortByteArrays(keys)
	for _, key := range keys {
		values = append(values, db[string(key)])
	}
	return &Itr{
		index:   -1,
		decoder: d,
		keys:    keys,
		values:  values,
	}, nil
}

func (it *Itr) first() {
	if len(it.keys) > 0 {
		it.index = 0
	}
}

func (it *Itr) last() {
	if len(it.keys) > 0 {
		it.index = len(it.keys) - 1
	}
}

func (it *Itr) seek(key []byte) {
	it.index = -1
	idx := 0
	for _, k := range it.keys {
		if bytes.Compare(k, key) >= 0 {
			break
		}
		idx++
	}
	// found a close or exact match
	if idx < len(it.keys) {
		it.index = idx
	}
}

func sortByteArrays(src [][]byte) {
	sort.Slice(src, func(i, j int) bool { return bytes.Compare(src[i], src[j]) < 0 })
}
