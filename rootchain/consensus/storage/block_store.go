package storage

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
)

type (
	BlockStore struct {
		hash          crypto.Hash // hash algorithm
		blockTree     *BlockTree
		storage       keyvaluedb.KeyValueDB
		orchestration Orchestration
		lock          sync.RWMutex
	}

	partitionShard struct {
		partition types.PartitionID
		shard     string // types.ShardID is not comparable
	}

	Orchestration interface {
		ShardEpoch(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error)
		ShardConfig(partition types.PartitionID, shard types.ShardID, epoch uint64) (*partitions.ValidatorAssignmentRecord, error)
		PartitionDescription(partition types.PartitionID, epoch uint64) (*types.PartitionDescriptionRecord, error)
	}
)

func storeGenesisInit(hash crypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB, orchestration Orchestration) error {
	genesisBlock, err := NewGenesisBlock(hash, pg, orchestration)
	if err != nil {
		return fmt.Errorf("creating genesis block: %w", err)
	}
	if err := db.Write(blockKey(genesisBlock.GetRound()), genesisBlock); err != nil {
		return fmt.Errorf("persist genesis block: %w", err)
	}
	return nil
}

func New(hash crypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB, orchestration Orchestration) (block *BlockStore, err error) {
	// Initiate store
	if pg == nil {
		return nil, errors.New("genesis record is nil")
	}
	if db == nil {
		return nil, errors.New("storage is nil")
	}
	// First start, initiate from genesis data
	empty, err := keyvaluedb.IsEmpty(db)
	if err != nil {
		return nil, fmt.Errorf("failed to read block store: %w", err)
	}
	if empty {
		if err = storeGenesisInit(hash, pg, db, orchestration); err != nil {
			return nil, fmt.Errorf("initializing block store: %w", err)
		}
	}

	blTree, err := NewBlockTree(db, orchestration)
	if err != nil {
		return nil, fmt.Errorf("initializing block tree: %w", err)
	}
	return &BlockStore{
		hash:          hash,
		blockTree:     blTree,
		storage:       db,
		orchestration: orchestration,
	}, nil
}

func NewFromState(hash crypto.Hash, stateMsg *abdrc.StateMsg, db keyvaluedb.KeyValueDB, orchestration Orchestration) (*BlockStore, error) {
	if db == nil {
		return nil, errors.New("storage is nil")
	}

	rootNode, err := NewRootBlock(hash, stateMsg.CommittedHead, orchestration)
	if err != nil {
		return nil, fmt.Errorf("failed to create new root node: %w", err)
	}

	blTree, err := NewBlockTreeFromRecovery(rootNode, db)
	if err != nil {
		return nil, fmt.Errorf("creating block tree from recovery: %w", err)
	}
	return &BlockStore{
		hash:          hash,
		blockTree:     blTree,
		storage:       db,
		orchestration: orchestration,
	}, nil
}

func (x *BlockStore) ProcessTc(tc *drctypes.TimeoutCert) (rErr error) {
	if tc == nil {
		return fmt.Errorf("error tc is nil")
	}
	// persist last known TC
	if err := WriteLastTC(x.storage, tc); err != nil {
		// store DB error and continue
		rErr = fmt.Errorf("TC write failed: %w", err)
	}
	// Remove proposal/block for TC round if it exists, since quorum voted for timeout.
	// It will never be committed, hence it can be removed immediately.
	// It is fine if the block is not found, it does not matter anyway
	if err := x.blockTree.RemoveLeaf(tc.GetRound()); err != nil {
		return errors.Join(rErr, fmt.Errorf("removing timeout block %v: %w", tc.GetRound(), err))
	}
	return rErr
}

// IsChangeInProgress - return input record if sysID has a pending IR change in the pipeline or nil if no change is
// currently in the pipeline.
func (x *BlockStore) IsChangeInProgress(partition types.PartitionID, shard types.ShardID) *types.InputRecord {
	// go through the block we have and make sure that there is no change in progress for this partition id
	for _, b := range x.blockTree.GetAllUncommittedNodes() {
		if _, ok := b.Changed[partitionShard{partition, shard.Key()}]; ok {
			return b.CurrentIR.Find(partition).IR
		}
	}
	return nil
}

func (x *BlockStore) GetDB() keyvaluedb.KeyValueDB {
	return x.storage
}

func (x *BlockStore) ProcessQc(qc *drctypes.QuorumCert) ([]*certification.CertificationResponse, error) {
	if qc == nil {
		return nil, fmt.Errorf("qc is nil")
	}
	// if we have processed it already then skip (in case we are the next leader we have already handled the QC)
	if x.GetHighQc().GetRound() >= qc.GetRound() {
		// stale qc
		return nil, nil
	}
	// add Qc to block tree
	if err := x.blockTree.InsertQc(qc); err != nil {
		return nil, fmt.Errorf("failed to insert QC into block tree: %w", err)
	}
	// This QC does not serve as commit QC, then we are done
	if qc.GetRound() == genesis.RootRound || qc.LedgerCommitInfo.Hash == nil {
		// NB! exception, no commit for genesis round
		return nil, nil
	}
	// If the QC commits a state
	// committed block becomes the new root and nodes to old root are removed
	committedBlock, err := x.blockTree.Commit(qc)
	if err != nil {
		return nil, err
	}
	// generate certificates for all partitions that have changes in progress
	ucs, err := committedBlock.GenerateCertificates(qc)
	if err != nil {
		return nil, fmt.Errorf("commit block failed to generate certificates for round %v: %w", committedBlock.GetRound(), err)
	}
	return ucs, nil
}

// Add adds new round state to pipeline and returns the new state root hash a.k.a. execStateID
func (x *BlockStore) Add(block *drctypes.BlockData, verifier IRChangeReqVerifier) ([]byte, error) {
	// verify that block for the round does not exist yet
	// if block already exists, then check that it is the same block by comparing block hash
	if b, err := x.blockTree.FindBlock(block.GetRound()); err == nil && b != nil {
		b1h := b.BlockData.Hash(crypto.SHA256)
		b2h := block.Hash(crypto.SHA256)
		// ignore if it is the same block, recovery may have added it when state was duplicated
		if bytes.Equal(b1h, b2h) {
			return b.RootHash, nil
		}
		return nil, fmt.Errorf("add block failed: different block for round %v is already in store", block.Round)
	}
	// block was not present, check parent block (QC round) is stored (if not node needs to recover)
	parentBlock, err := x.blockTree.FindBlock(block.GetParentRound())
	if err != nil {
		return nil, fmt.Errorf("add block failed: parent round %v not found, recover", block.Qc.VoteInfo.RoundNumber)
	}
	// Extend state from parent block
	exeBlock, err := parentBlock.Extend(x.hash, block, verifier, x.orchestration)
	if err != nil {
		return nil, fmt.Errorf("error processing block round %v, %w", block.Round, err)
	}
	// append new block
	if err = x.blockTree.Add(exeBlock); err != nil {
		return nil, fmt.Errorf("adding block to the tree: %w", err)
	}
	return exeBlock.RootHash, nil
}

func (x *BlockStore) GetHighQc() *drctypes.QuorumCert {
	return x.blockTree.HighQc()
}

func (x *BlockStore) GetLastTC() (*drctypes.TimeoutCert, error) {
	return ReadLastTC(x.storage)
}

func (x *BlockStore) GetCertificate(id types.PartitionID, shard types.ShardID) (*certification.CertificationResponse, error) {
	x.lock.RLock()
	defer x.lock.RUnlock()

	committedBlock := x.blockTree.Root()
	if si, ok := committedBlock.ShardInfo[partitionShard{partition: id, shard: shard.Key()}]; ok {
		return si.LastCR, nil
	}
	return nil, fmt.Errorf("no certificate found for shard %s - %s", id, shard)
}

func (x *BlockStore) GetCertificates() []*types.UnicityCertificate {
	x.lock.RLock()
	defer x.lock.RUnlock()

	committedBlock := x.blockTree.Root()
	ucs := make([]*types.UnicityCertificate, 0, len(committedBlock.ShardInfo))
	for _, v := range committedBlock.ShardInfo {
		ucs = append(ucs, &v.LastCR.UC)
	}
	return ucs
}

func (x *BlockStore) ShardInfo(partition types.PartitionID, shard types.ShardID) (*ShardInfo, error) {
	x.lock.RLock()
	defer x.lock.RUnlock()

	committedBlock := x.blockTree.Root()
	if si, ok := committedBlock.ShardInfo[partitionShard{partition, shard.Key()}]; ok {
		return si, nil
	}
	return nil, fmt.Errorf("no shard info found for {%s : %s}", partition, shard)
}

func (x *BlockStore) GetState() (*abdrc.StateMsg, error) {
	return x.blockTree.CurrentState()
}

/*
Block returns block for given round.
When store doesn't have block for the round it returns error.
*/
func (x *BlockStore) Block(round uint64) (*ExecutedBlock, error) {
	return x.blockTree.FindBlock(round)
}

// StoreLastVote stores last sent vote message by this node
func (x *BlockStore) StoreLastVote(vote any) error {
	return WriteVote(x.storage, vote)
}

// ReadLastVote returns last sent vote message by this node
func (x *BlockStore) ReadLastVote() (any, error) {
	return ReadVote(x.storage)
}
