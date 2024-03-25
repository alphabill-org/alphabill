package wvm

import (
	"github.com/alphabill-org/alphabill/wvm/allocator"
	"github.com/tetratelabs/wazero/api"
)

type WazeroMemory struct {
	mem api.Memory
}

func (w WazeroMemory) Size() uint32 {
	return w.mem.Size()
}

func (w WazeroMemory) Max() uint32 {
	if maxPages, encoded := w.mem.Definition().Max(); encoded {
		return maxPages
	}
	return allocator.MaxPages
}

func (w WazeroMemory) ReadUint64Le(offset uint32) (uint64, bool) {
	return w.mem.ReadUint64Le(offset)
}

func (w WazeroMemory) WriteUint64Le(offset uint32, v uint64) bool {
	return w.mem.WriteUint64Le(offset, v)
}

func (w WazeroMemory) Grow(deltaPages uint32) (previousPages uint32, ok bool) {
	return w.mem.Grow(deltaPages)
}

func NewWazeroMemoryWrapper(memory api.Memory) *WazeroMemory {
	return &WazeroMemory{mem: memory}
}
