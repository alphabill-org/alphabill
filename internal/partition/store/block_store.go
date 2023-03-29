package store

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
)

// BlockStore provides methods to store and query blockchain blocks.
type BlockStore interface {
	// Add adds the new block to the blockchain.
	Add(b *block.Block) error
	// AddGenesis persists the genesis block, this is the only time when a block can be empty
	AddGenesis(b *block.Block) error
	// Get returns the block with given number, or nil if not found.
	Get(blockNumber uint64) (*block.Block, error)
	// BlockNumber returns the round number of the latest non-empty committed blocks in the blockchain.
	BlockNumber() (uint64, error)
	// LatestRoundNumber returns the latest known certified round number
	LatestRoundNumber() (uint64, error)
	// LatestBlock returns the latest committed (non-empty) block.
	LatestBlock() (*block.Block, error)
	// LatestUC returns the latest known unicity certificate (its round number might be larger than the last persisted block).
	LatestUC() (*certificates.UnicityCertificate, error)
	// SetPendingProposal stores the pending block proposal, nil removed stored proposal.
	SetPendingProposal(proposal *block.PendingBlockProposal) error
	// GetPendingProposal returns the pending block proposal if it exists.
	GetPendingProposal() (*block.PendingBlockProposal, error)
}
