package bumpallocator

import (
	"fmt"
	"math"
)

const (
	WasmPageSize = 1 << 16 // 64Kb
	MaxPages     = 4 * 1024 * 1024 * 1024 / WasmPageSize
	Alignment    = 8
)

type statistics struct {
	allocCount    uint64
	freeCount     uint64
	allocDataSize uint64
}

type BumpAllocator struct {
	heapBase     uint32
	freePtr      uint32
	arenaSize    uint32
	memPageLimit uint32
	stats        statistics
	errState     error
}

func maxPages(info MemInfo) uint32 {
	if maxPages, encoded := info.Max(); encoded {
		return maxPages
	}
	return MaxPages
}

// align - aligns address to next multiple of 8 - i.e. 8 byte alignment
// todo: check if wasm has explicit memory alignment requirements, perhaps could use no alignment - less wasted memory
func align(addr uint64) uint64 {
	return ((addr + Alignment - 1) / Alignment) * Alignment
}

func addrToPage(addr uint64) (uint32, error) {
	// round up
	pageNo := (addr + uint64(WasmPageSize) - 1) / uint64(WasmPageSize)
	if pageNo > uint64(MaxPages) {
		return 0, fmt.Errorf("out of memory pages")
	}
	return uint32(pageNo), nil
}

func New(heapBase uint32, info MemInfo) *BumpAllocator {
	return &BumpAllocator{
		heapBase:     heapBase,
		freePtr:      heapBase,
		memPageLimit: maxPages(info),
		stats:        statistics{},
	}
}

func (b *BumpAllocator) Alloc(mem Memory, size uint32) (ptr uint32, err error) {
	if b.errState != nil {
		return 0, b.errState
	}
	// If any error occurs, put the allocator in error state, it can no longer be trusted to work correctly.
	defer func() {
		if err != nil {
			b.errState = err
		}
	}()
	if err = b.monitorArenaSize(mem.Size()); err != nil {
		return 0, err
	}
	return b.bumpAlloc(size, mem)
}

func (b *BumpAllocator) HeapBase() uint32 {
	return b.heapBase
}

func (b *BumpAllocator) Free(_ Memory, _ uint32) error {
	if b.errState != nil {
		return b.errState
	}
	b.stats.freeCount++
	return nil
}

func (b *BumpAllocator) monitorArenaSize(currentSize uint32) error {
	if b.arenaSize > currentSize && b.freePtr > currentSize {
		return fmt.Errorf("memory arena has shrunk unexpectedly from %v to %v", b.arenaSize, currentSize)
	}
	b.arenaSize = currentSize
	return nil
}

func (b *BumpAllocator) bumpAlloc(size uint32, mem Memory) (uint32, error) {
	newFreePtr := align(uint64(b.freePtr) + uint64(size))
	if newFreePtr > math.MaxUint32 {
		return 0, fmt.Errorf("out of memory")
	}
	// do we need to grow the arena?
	if uint64(b.arenaSize) < newFreePtr {
		requiredPages, err := addrToPage(newFreePtr)
		if err != nil {
			return 0, fmt.Errorf("memory page allocation error: %w", err)
		}
		currentPages := mem.Size() / WasmPageSize
		// if possible, then double, if not grow to maximum pages
		incPages := min(currentPages*2, b.memPageLimit)
		incPages = max(incPages, requiredPages)
		// call Grow with diff a.k.a how many more we need
		_, ok := mem.Grow(incPages - currentPages)
		if !ok {
			return 0, fmt.Errorf("linear memory grow error: from %d pages to %d pages", currentPages, incPages)
		}
		b.arenaSize = mem.Size()
		// validation check, maybe remove later
		if (b.arenaSize / WasmPageSize) != incPages {
			return 0, fmt.Errorf("number of pages should have increased! previous: %d, desired: %d", currentPages, incPages)
		}
	}
	b.stats.allocCount++
	// monitor data size too, so it will be possible to calculate the wasted bytes
	b.stats.allocDataSize += uint64(size)
	addr := b.freePtr
	b.freePtr = uint32(newFreePtr)
	return addr, nil
}
