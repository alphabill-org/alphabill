package store

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
)

// InMemoryBlockStore is an in-memory implementation of BlockStore interface.
type InMemoryBlockStore struct {
	blocks map[uint64]*block.Block
}

func NewInMemoryBlockStore() *InMemoryBlockStore {
	return &InMemoryBlockStore{blocks: map[uint64]*block.Block{}}
}

func (bs *InMemoryBlockStore) Add(b *block.Block) error {
	bs.blocks[b.TxSystemBlockNumber] = b
	return nil
}

func (bs *InMemoryBlockStore) Get(blockNumber uint64) (*block.Block, error) {
	return bs.blocks[blockNumber], nil
}

func (bs *InMemoryBlockStore) Height() (uint64, error) {
	return uint64(len(bs.blocks)), nil
}

func (bs *InMemoryBlockStore) LatestBlock() (*block.Block, error) {
	height, err := bs.Height()
	if err != nil {
		return nil, err
	}
	return bs.blocks[height], nil
}
