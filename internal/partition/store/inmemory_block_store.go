package store

import (
	"sync"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
)

// InMemoryBlockStore is an in-memory implementation of BlockStore interface.
type InMemoryBlockStore struct {
	mu                   sync.RWMutex
	blocks               map[uint64]*block.Block
	pendingBlockProposal *block.PendingBlockProposal
}

func NewInMemoryBlockStore() *InMemoryBlockStore {
	return &InMemoryBlockStore{blocks: map[uint64]*block.Block{}}
}

func (bs *InMemoryBlockStore) Add(b *block.Block) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.blocks[b.BlockNumber] = b
	return nil
}

func (bs *InMemoryBlockStore) Get(blockNumber uint64) (*block.Block, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.blocks[blockNumber], nil
}

func (bs *InMemoryBlockStore) Height() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return uint64(len(bs.blocks)), nil
}

func (bs *InMemoryBlockStore) LatestBlock() *block.Block {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.blocks[uint64(len(bs.blocks))]
}

func (bs *InMemoryBlockStore) AddPendingProposal(proposal *block.PendingBlockProposal) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.pendingBlockProposal = proposal
	return nil
}

func (bs *InMemoryBlockStore) GetPendingProposal() (*block.PendingBlockProposal, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	if bs.pendingBlockProposal == nil {
		return nil, errors.New(ErrStrPendingBlockProposalNotFound)
	}
	return bs.pendingBlockProposal, nil
}
