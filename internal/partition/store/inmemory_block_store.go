package store

import (
	"sync"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/partition/genesis"
)

// InMemoryBlockStore is an in-memory implementation of BlockStore interface.
type InMemoryBlockStore struct {
	mu                   sync.RWMutex
	latestUC             *certificates.UnicityCertificate
	latestBlockNumber    uint64 // most recent persisted non-empty block
	blocks               map[uint64]*block.Block
	pendingBlockProposal *block.PendingBlockProposal
}

func NewInMemoryBlockStore() *InMemoryBlockStore {
	logger.Info("Creating InMemoryBlockStore")
	return &InMemoryBlockStore{blocks: map[uint64]*block.Block{}}
}

func (bs *InMemoryBlockStore) Add(b *block.Block) error {
	return bs.add(b, false)
}

func (bs *InMemoryBlockStore) AddGenesis(b *block.Block) error {
	if b.UnicityCertificate.InputRecord.RoundNumber != genesis.GenesisRoundNumber {
		return errors.Errorf("genesis block round number should be %d", genesis.GenesisRoundNumber)
	}
	return bs.add(b, true)
}

func (bs *InMemoryBlockStore) add(b *block.Block, allowEmpty bool) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	latestRoundNumber := b.UnicityCertificate.InputRecord.RoundNumber
	bs.latestUC = b.UnicityCertificate
	if !allowEmpty && len(b.Transactions) == 0 {
		return nil
	}
	bs.latestBlockNumber = latestRoundNumber
	bs.blocks[latestRoundNumber] = b
	return nil
}

func (bs *InMemoryBlockStore) Get(blockNumber uint64) (*block.Block, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.blocks[blockNumber], nil
}

func (bs *InMemoryBlockStore) LatestRoundNumber() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.latestUC.InputRecord.RoundNumber, nil
}

func (bs *InMemoryBlockStore) BlockNumber() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.latestBlockNumber, nil
}

func (bs *InMemoryBlockStore) LatestBlock() (*block.Block, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.blocks[bs.latestBlockNumber], nil
}

func (bs *InMemoryBlockStore) LatestUC() (*certificates.UnicityCertificate, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.latestUC, nil
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
