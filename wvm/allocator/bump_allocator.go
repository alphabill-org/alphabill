package allocator

import (
	"fmt"
	"math"
)

const (
	WasmPageSize = 65536
	MaxPages     = 4 * 1024 * 1024 * 1024 / WasmPageSize
	Alignment    = 8
)

type statistics struct {
	allocCount    uint64
	freeCount     uint64
	allocDataSize uint64
}

type BumpAllocator struct {
	heapBase  uint32
	freePtr   uint32
	arenaSize uint32
	stats     statistics
	errState  error
}

// align - aligns address to next multiple of 8 - i.e. 8 byte alignment
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

func NewBumpAllocator(heapBase uint32) *BumpAllocator {
	return &BumpAllocator{
		heapBase: heapBase,
		freePtr:  heapBase,
		stats:    statistics{},
	}
}

func (f *BumpAllocator) Alloc(mem Memory, size uint32) (ptr uint32, err error) {
	if f.errState != nil {
		return 0, f.errState
	}
	// If any error occurs, put the allocator in error state, it can no longer be trusted to work correctly.
	defer func() {
		if err != nil {
			f.errState = err
		}
	}()
	if err = f.monitorArenaSize(mem.Size()); err != nil {
		return 0, err
	}
	return f.bumpAlloc(size, mem)
}

func (f *BumpAllocator) Free(_ Memory, _ uint32) (err error) {
	if f.errState != nil {
		return f.errState
	}
	f.stats.freeCount++
	return nil
}

func (f *BumpAllocator) monitorArenaSize(currSize uint32) error {
	if f.arenaSize > currSize && f.freePtr > currSize {
		return fmt.Errorf("memory arena has shrunk unexpectedly from %v to %v", f.arenaSize, currSize)
	}
	f.arenaSize = currSize
	return nil
}

func (f *BumpAllocator) bumpAlloc(size uint32, mem Memory) (uint32, error) {
	newFreePtr := align(uint64(f.freePtr) + uint64(size))
	if newFreePtr > math.MaxUint32 {
		return 0, fmt.Errorf("out of memory")
	}
	// do we need to grow the arena?
	if uint64(f.arenaSize) < newFreePtr {
		requiredPages, err := addrToPage(newFreePtr)
		if err != nil {
			return 0, fmt.Errorf("memory page allocation error: %w", err)
		}
		currentPages := mem.Size() / WasmPageSize
		// if we need to grow then let's at least double
		incPages := min(currentPages*2, MaxWasmPages)
		incPages = max(incPages, requiredPages)
		// call Grow with diff a.k.a how many more we need
		_, ok := mem.Grow(incPages - currentPages)
		if !ok {
			return 0, fmt.Errorf("%w: from %d pages to %d pages",
				ErrCannotGrowLinearMemory, currentPages, incPages)
		}
		f.arenaSize = mem.Size()
		// validation check, maybe remove later
		if (f.arenaSize / PageSize) != incPages {
			return 0, fmt.Errorf("number of pages should have increased! previous: %d, desired: %d", currentPages, incPages)
		}
	}
	f.stats.allocCount++
	f.stats.allocDataSize += uint64(size)
	addr := f.freePtr
	f.freePtr = uint32(newFreePtr)
	return addr, nil
}
