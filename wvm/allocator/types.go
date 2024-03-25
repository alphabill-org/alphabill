package allocator

type LinearMemory interface {
	Size() uint32
	Max() uint32
	ReadUint64Le(offset uint32) (uint64, bool)
	WriteUint64Le(offset uint32, v uint64) bool
	Grow(deltaPages uint32) (previousPages uint32, ok bool)
}
