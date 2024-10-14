package bumpallocator

type Memory interface {
	Size() uint32
	Read(offset, cnt uint32) ([]byte, bool)
	Write(offset uint32, data []byte) bool
	Grow(deltaPages uint32) (previousPages uint32, ok bool)
}

type MemInfo interface {
	Max() (uint32, bool)
}
