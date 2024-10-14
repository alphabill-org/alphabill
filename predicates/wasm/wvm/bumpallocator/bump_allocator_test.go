package bumpallocator

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/util"
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
		p, err = addrToPage(uint64(WasmPageSize) * MaxPages)
		require.NoError(t, err)
		require.EqualValues(t, MaxPages, p)
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
	allocator := New(0, mem.Definition())
	require.EqualValues(t, 0, allocator.HeapBase())
	ptr1, err := allocator.Alloc(mem, 1)
	require.NoError(t, err)
	require.EqualValues(t, 0, ptr1)
	freePtr := allocator.freePtr
	// check alignment
	require.EqualValues(t, 0, freePtr%8)
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
	require.NoError(t, err)
	require.EqualValues(t, freePtr, ptr3)
	freePtr = allocator.freePtr
	require.EqualValues(t, freePtr, 3*WasmPageSize+8)
	require.NoError(t, allocator.Free(mem, ptr3))
	require.NoError(t, allocator.Free(mem, ptr2))
	require.EqualValues(t, 0, allocator.stats.allocCount-allocator.stats.freeCount)
}

func TestMemoryArenaChange(t *testing.T) {
	mem := NewMemoryMock(t, 1)
	allocator := New(8, mem.Definition())
	require.EqualValues(t, 8, allocator.HeapBase())
	// start allocating from the end of stack space
	require.EqualValues(t, allocator.freePtr, allocator.HeapBase())
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
	const pageLimit = 4
	mem := NewMemoryMockWithLimit(t, 1, pageLimit)
	allocator := New(8, mem.Definition())
	// allocate 1024 Kb
	ptr, err := allocator.Alloc(mem, 1024)
	require.NoError(t, err)
	require.NoError(t, allocator.Free(mem, ptr))
	require.EqualValues(t, 1, allocator.arenaSize/WasmPageSize)
	// allocate 1 full page - nof pages required is doubled and is now 2
	ptr, err = allocator.Alloc(mem, WasmPageSize)
	require.NoError(t, err)
	require.NoError(t, allocator.Free(mem, ptr))
	require.EqualValues(t, 2, allocator.arenaSize/WasmPageSize)
	// allocate another page - nof pages are doubled again and is now 4
	ptr, err = allocator.Alloc(mem, WasmPageSize)
	require.NoError(t, err)
	require.NoError(t, allocator.Free(mem, ptr))
	require.EqualValues(t, 4, allocator.arenaSize/WasmPageSize)
	// allocate another page - no need to allocate more we have enough at this point
	ptr, err = allocator.Alloc(mem, WasmPageSize)
	require.NoError(t, err)
	require.NoError(t, allocator.Free(mem, ptr))
	require.EqualValues(t, 4, allocator.arenaSize/WasmPageSize)
	// can allocate 1Kb - we would need 5 pages, but max is set to 4 so this will fail
	ptr, err = allocator.Alloc(mem, 1024)
	require.NoError(t, err)
	require.NotZero(t, ptr)
	ptr, err = allocator.Alloc(mem, WasmPageSize)
	require.EqualError(t, err, "linear memory grow error: from 4 pages to 5 pages")
	require.EqualValues(t, 0, ptr)
}

func TestReadWrite(t *testing.T) {
	mem := NewMemoryMock(t, 1)
	allocator := New(8, mem.Definition())
	// allocate 1024 Kb
	ptr, err := allocator.Alloc(mem, 8)
	require.NoError(t, err)
	mem.Write(ptr, util.Uint64ToBytes(0xaabbccddee))
	data, ok := mem.Read(ptr, 8)
	require.True(t, ok)
	require.EqualValues(t, 0xaabbccddee, util.BytesToUint64(data))
}
