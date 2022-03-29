package partition

// InMemoryBlockStore is an in-memory implementation of BlockStore interface.
type InMemoryBlockStore struct {
	blocks []*Block
}

func (bs *InMemoryBlockStore) Add(b *Block) {
	bs.blocks = append(bs.blocks, b)
}

func (bs *InMemoryBlockStore) Height() uint64 {
	return uint64(len(bs.blocks))
}

func (bs *InMemoryBlockStore) LatestBlock() *Block {
	return bs.blocks[bs.Height()-1]
}
