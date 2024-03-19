package allocator

import (
	"encoding/binary"
	"testing"
)

type MemoryMockup struct {
	data         []byte
	maxWasmPages uint32
}

func (m *MemoryMockup) setMaxWasmPages(max uint32) {
	m.maxWasmPages = max
}

func (m *MemoryMockup) Max() uint32 {
	return m.maxWasmPages
}

func (m *MemoryMockup) pages() uint32 {
	return uint32((uint64(len(m.data)) + uint64(WasmPageSize) - 1) / uint64(WasmPageSize))
}

func (m *MemoryMockup) Size() uint32 {
	return m.pages() * WasmPageSize
}

func (m *MemoryMockup) Grow(pages uint32) (uint32, bool) {
	if m.pages()+pages > m.maxWasmPages {
		return 0, false
	}
	prevPages := m.pages()
	resizedLinearMem := make([]byte, (prevPages+pages)*WasmPageSize)
	copy(resizedLinearMem[0:len(m.data)], m.data)
	m.data = resizedLinearMem
	return prevPages, true
}

func (m *MemoryMockup) ReadUint64Le(offset uint32) (uint64, bool) {
	return binary.LittleEndian.Uint64(m.data[offset : offset+8]), true
}

func (m *MemoryMockup) WriteUint64Le(offset uint32, v uint64) bool {
	encoded := make([]byte, 8)
	binary.LittleEndian.PutUint64(encoded, v)
	copy(m.data[offset:offset+8], encoded)
	return true
}

func NewMemoryMock(t *testing.T, pages uint32) *MemoryMockup {
	t.Helper()
	return &MemoryMockup{
		data:         make([]byte, pages*WasmPageSize),
		maxWasmPages: MaxPages,
	}
}
