package allocator

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_addrToPage(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		p, err := addrToPage(0)
		require.NoError(t, err)
		require.EqualValues(t, 0, p)
		p, err = addrToPage(1)
		require.NoError(t, err)
		require.EqualValues(t, 1, p)
		p, err = addrToPage(uint64(WasmPageSize))
		require.NoError(t, err)
		require.EqualValues(t, 1, p)
		p, err = addrToPage(uint64(WasmPageSize + 1))
		require.NoError(t, err)
		require.EqualValues(t, 2, p)
		p, err = addrToPage(uint64(WasmPageSize * 2))
		require.NoError(t, err)
		require.EqualValues(t, 2, p)
		p, err = addrToPage(uint64(WasmPageSize*4 + 1))
		require.NoError(t, err)
		require.EqualValues(t, 5, p)
		p, err = addrToPage(uint64(WasmPageSize) * MaxWasmPages)
		require.NoError(t, err)
		require.EqualValues(t, MaxWasmPages, p)
	})
	t.Run("error", func(t *testing.T) {
		p, err := addrToPage(uint64(4*1024*1024*1024 + 1))
		require.Error(t, err)
		require.EqualValues(t, 0, p)
	})
}

func Test_align(t *testing.T) {
	require.EqualValues(t, 0, align(0))
	require.EqualValues(t, 8, align(1))
	require.EqualValues(t, 8, align(7))
	require.EqualValues(t, 8, align(8))
	require.EqualValues(t, 16, align(9))
}

func TestAllocate(t *testing.T) {
	mem := NewMemoryMock(t, 1)
	allocator := NewBumpAllocator(0)

	ptr1, err := allocator.Alloc(mem, 1)
	require.NoError(t, err)
	require.EqualValues(t, 0, ptr1)
	freePtr := allocator.freePtr
	require.EqualValues(t, freePtr, 8)
	require.EqualValues(t, allocator.stats.allocCount, 1)
	require.EqualValues(t, allocator.stats.allocDataSize, 1)
	require.NoError(t, allocator.Free(mem, ptr1))
	require.EqualValues(t, 0, allocator.stats.allocCount-allocator.stats.freeCount)
	// requires a new page
	ptr2, err := allocator.Alloc(mem, WasmPageSize)
	require.NoError(t, err)
	// free memory started from address 8
	require.EqualValues(t, freePtr, ptr2)
	freePtr = allocator.freePtr
	require.EqualValues(t, freePtr, WasmPageSize+8)
	// allocate another 2*pages
	ptr3, err := allocator.Alloc(mem, WasmPageSize*2)
	require.EqualValues(t, freePtr, ptr3)
	freePtr = allocator.freePtr
	require.EqualValues(t, freePtr, 3*WasmPageSize+8)
	require.NoError(t, allocator.Free(mem, ptr3))
	require.NoError(t, allocator.Free(mem, ptr2))
	require.EqualValues(t, 0, allocator.stats.allocCount-allocator.stats.freeCount)
}

func TestMemoryArenaChange(t *testing.T) {
	mem := NewMemoryMock(t, 1)
	allocator := NewBumpAllocator(8)
	ptr, err := allocator.Alloc(mem, 2*WasmPageSize+13)
	require.NoError(t, err)
	require.EqualValues(t, 8, ptr)
	freePtr := allocator.freePtr
	// aligned to next 8 multiple - 16
	require.EqualValues(t, freePtr, 8+2*WasmPageSize+16)
	// force error
	mem.data = make([]byte, 1*WasmPageSize)
	// try to allocate again
	ptr2, err := allocator.Alloc(mem, 8)
	require.EqualError(t, err, fmt.Sprintf("memory arena has shrunk unexpectedly from %v to %v", 3*WasmPageSize, WasmPageSize))
	require.EqualValues(t, 0, ptr2)
	// allocator is now in error state, all next allocations will fail too
	ptr3, err := allocator.Alloc(mem, 3)
	require.Error(t, err)
	require.EqualValues(t, 0, ptr3)
	// free will also fail
	require.Error(t, allocator.Free(mem, ptr))
}

func TestAllocateOverLimit(t *testing.T) {
	mem := NewMemoryMock(t, 1)
	mem.setMaxWasmPages(4)
	allocator := NewBumpAllocator(8)
	for i := 0; i < 3; i++ {
		ptr, err := allocator.Alloc(mem, WasmPageSize)
		require.NoError(t, err)
		require.NoError(t, allocator.Free(mem, ptr))
	}
	ptr, err := allocator.Alloc(mem, WasmPageSize)
	require.EqualError(t, err, "cannot grow linear memory: from 4 pages to 8 pages")
	require.EqualValues(t, 0, ptr)
}
