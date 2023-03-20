package storage

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill/internal/keyvaleudb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	// tree node creates a chain of consecutive blocks
	node struct {
		data  *ExecutedBlock
		child *node // add by view number
	}

	BlockTree struct {
		root        *node
		roundToNode map[uint64]*node
		highQc      *atomic_broadcast.QuorumCert
		blocksDB    keyvaleudb.KeyValueDB
	}
)

func newNode(b *ExecutedBlock) *node {
	return &node{data: b, child: nil}
}

func (l *node) addChild(child *node) error {
	if l.child != nil {
		return fmt.Errorf("block already with child")
	}
	l.child = child
	return nil
}

func (l *node) removeChild() {
	l.child = nil
}

func blockStoreGenesisInit(genesisBlock *ExecutedBlock, blocks keyvaleudb.KeyValueDB) error {
	if err := blocks.Write(util.Uint64ToBytes(genesisBlock.BlockData.Round), genesisBlock); err != nil {
		return fmt.Errorf("genesis block write failed, %w", err)
	}
	return nil
}

func NewBlockTreeFromRecovery(cBlock *ExecutedBlock, nodes []*ExecutedBlock, bDB keyvaleudb.KeyValueDB) (*BlockTree, error) {
	rootNode := newNode(cBlock)
	hQC := rootNode.data.Qc
	// make sure that nodes are sorted by round
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].BlockData.Round < nodes[j].BlockData.Round
	})
	treeNodes := map[uint64]*node{rootNode.data.BlockData.Round: rootNode}
	for _, next := range nodes {
		// if parent round does not exist then reject, parent must be recovered
		parent, found := treeNodes[next.BlockData.Qc.VoteInfo.RoundNumber]
		if found == false {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block %v not found", next.BlockData.Round,
				next.BlockData.Qc.VoteInfo.RoundNumber)
		}
		// append block and add a child to parent
		n := newNode(next)
		treeNodes[next.BlockData.Round] = n
		if err := parent.addChild(n); err != nil {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block add child error, %w", next.BlockData.Round, err)
		}
		if n.data.Qc != nil {
			hQC = n.data.Qc
		}
	}
	return &BlockTree{
		roundToNode: treeNodes,
		root:        rootNode,
		highQc:      hQC,
		blocksDB:    bDB,
	}, nil
}

func NewBlockTree(bDB keyvaleudb.KeyValueDB) (*BlockTree, error) {
	if bDB == nil {
		return nil, fmt.Errorf("block tree init failed, databes is nil")
	}
	itr := bDB.Last()
	defer func() {
		if err := itr.Close(); err != nil {
			logger.Warning("Unexpected error, db iterator close %v", err)
		}
	}()
	var blocks []*ExecutedBlock
	var lastRoot *ExecutedBlock = nil
	var hQC *atomic_broadcast.QuorumCert = nil
	for ; itr.Valid(); itr.Prev() {
		round := util.BytesToUint64(itr.Key())
		var b ExecutedBlock
		if err := itr.Value(&b); err != nil {
			return nil, fmt.Errorf("read block %v from db failed, %w", round, err)
		}
		// read until latest committed block is found
		if b.CommitQc != nil && b.CommitQc.LedgerCommitInfo != nil {
			lastRoot = &b
			break
		}
		blocks = append(blocks, &b)
	}
	if lastRoot == nil {
		return nil, fmt.Errorf("block tree init failed to recover latest committed block")
	}
	// make sure that nodes are sorted by round
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].BlockData.Round < blocks[j].BlockData.Round
	})
	rootNode := newNode(lastRoot)
	hQC = rootNode.data.Qc
	treeNodes := map[uint64]*node{lastRoot.BlockData.Round: rootNode}
	for _, next := range blocks {
		// if parent round does not exist then reject, parent must be recovered
		parent, found := treeNodes[next.BlockData.Qc.VoteInfo.RoundNumber]
		if found == false {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block %v not found", next.BlockData.Round,
				next.BlockData.Qc.VoteInfo.RoundNumber)
		}
		// append block and add a child to parent
		n := newNode(next)
		treeNodes[next.BlockData.Round] = n
		if err := parent.addChild(n); err != nil {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block add child error, %w", next.BlockData.Round, err)
		}
		if n.data.Qc != nil {
			hQC = n.data.Qc
		}
	}
	return &BlockTree{
		roundToNode: treeNodes,
		root:        rootNode,
		highQc:      hQC,
		blocksDB:    bDB,
	}, nil
}

func (bt *BlockTree) InsertQc(qc *atomic_broadcast.QuorumCert, bockDB keyvaleudb.KeyValueDB) error {
	// find block, if it does not exist, return error we need to recover missing info
	b, err := bt.FindBlock(qc.VoteInfo.RoundNumber)
	if err != nil {
		return fmt.Errorf("block tree add qc failed, %w", err)
	}
	if !bytes.Equal(b.RootHash, qc.VoteInfo.CurrentRootHash) {
		return fmt.Errorf("block tree add qc failed, qc state hash is different from local computed state hash")
	}
	b.Qc = qc
	// persist changes
	if err = bockDB.Write(util.Uint64ToBytes(b.BlockData.Round), b); err != nil {
		return fmt.Errorf("failed to persist block for round %v, %w", b.BlockData.Round, err)
	}
	bt.highQc = qc
	return nil
}

func (bt *BlockTree) HighQc() *atomic_broadcast.QuorumCert {
	return bt.highQc
}

// Add adds new leaf to the block tree
func (bt *BlockTree) Add(block *ExecutedBlock) error {
	// every round can exist only once
	// reject a block if this round has already been added
	if _, found := bt.roundToNode[block.BlockData.Round]; found == true {
		return fmt.Errorf("error block for round %v already exists", block.BlockData.Round)
	}
	// if parent round does not exist then reject, parent must be recovered
	parent, found := bt.roundToNode[block.BlockData.Qc.VoteInfo.RoundNumber]
	if found == false {
		return fmt.Errorf("error cannot add block for round %v, parent block %v not found", block.BlockData.Round,
			block.BlockData.Qc.VoteInfo.RoundNumber)
	}
	// append block and add a child to parent
	n := newNode(block)
	if err := parent.addChild(n); err != nil {
		return fmt.Errorf("error cannot add block for round %v, parent block add child error, %w", block.BlockData.Round, err)
	}
	bt.roundToNode[block.BlockData.Round] = n
	// persist block
	return bt.blocksDB.Write(util.Uint64ToBytes(block.BlockData.Round), n.data)
}

// RemoveLeaf removes leaf node if it is not root node
func (bt *BlockTree) RemoveLeaf(round uint64) error {
	// root cannot be removed
	if bt.root.data.BlockData.Round == round {
		return fmt.Errorf("error root cannot be removed")
	}
	n, found := bt.roundToNode[round]
	if found == false {
		logger.Debug("Remove node failed, node for round %v not found, perhaps already removed?", round)
		// this is ok if we do not have the node, on TC remove might be triggered twice
		return nil
	}
	if n.child != nil {
		return fmt.Errorf("error round %v is not child node", round)
	}
	parent, found := bt.roundToNode[n.data.BlockData.Qc.VoteInfo.RoundNumber]
	if found == false {
		return fmt.Errorf("error parent block %v not found", n.data.BlockData.Qc.VoteInfo.RoundNumber)
	}
	delete(bt.roundToNode, round)
	parent.removeChild()
	return bt.blocksDB.Delete(util.Uint64ToBytes(round))
}

func (bt *BlockTree) Root() *ExecutedBlock {
	return bt.root.data
}

// FindPathToRoot finds bath from node with round to root node
// returns nil in case of failure, when a node is not found
// otherwise it will return list of data stored in the path nodes, excluding the root data itself, so if
// the root node is previous node then the list is empty
func (bt *BlockTree) FindPathToRoot(round uint64) []*ExecutedBlock {
	n, f := bt.roundToNode[round]
	if f == false {
		return nil
	}
	// if the node is root
	if n == bt.root {
		return []*ExecutedBlock{}
	}
	// find path
	path := make([]*ExecutedBlock, 0, 2)
	for {
		if parent, found := bt.roundToNode[n.data.BlockData.Qc.VoteInfo.RoundNumber]; found == true {
			path = append(path, n.data)
			// if parent is root then break out of loop
			if parent == bt.root {
				break
			}
			n = parent
			continue
		}
		// node not found, should never happen, this is not a tree data structure then
		return nil
	}
	return path
}

func (bt *BlockTree) GetAllUncommittedNodes() []*ExecutedBlock {
	blocks := make([]*ExecutedBlock, 0, 2)
	n := bt.root.child
	for n != nil {
		blocks = append(blocks, n.data)
		n = n.child
	}
	return blocks
}

// findBlocksToPrune will return all blocks that can be removed from previous root to new root
// - In a normal case there is only one block, the previous root that will be pruned
// - In case of timeouts there can be more than one, the blocks in between old root and new
// are committed by the same QC.
func (bt *BlockTree) findBlocksToPrune(newRootRound uint64) ([]uint64, error) {
	blocksToPrune := make([]uint64, 0, 2)
	// nothing to be pruned
	if newRootRound == bt.root.data.BlockData.Round {
		return blocksToPrune, nil
	}
	treeNode := bt.root
	newRootFound := false
	for treeNode.child != nil {
		blocksToPrune = append(blocksToPrune, treeNode.data.BlockData.Round)
		// if the child is to become the new root then stop here
		if treeNode.child.data.BlockData.Round == newRootRound {
			newRootFound = true
			break
		}
		treeNode = treeNode.child
	}
	if newRootFound == false {
		return nil, fmt.Errorf("error, new root round %v not found", newRootRound)
	}
	return blocksToPrune, nil
}

func (bt *BlockTree) FindBlock(round uint64) (*ExecutedBlock, error) {
	b, found := bt.roundToNode[round]
	if found != true {
		return nil, fmt.Errorf("block for round %v not found", round)
	}
	return b.data, nil
}

// Commit commits block for round and prunes all preceding blocks from the tree,
// the committed block becomes the new root of the tree
func (bt *BlockTree) Commit(commitQc *atomic_broadcast.QuorumCert) (*ExecutedBlock, error) {
	// Add qc to pending state (needed for recovery)
	commitRound := commitQc.VoteInfo.ParentRoundNumber
	commitNode, found := bt.roundToNode[commitRound]
	if found == false {
		return nil, fmt.Errorf("commit of round %v failed, block not found", commitRound)
	}
	// Find if there are uncommitted nodes between new root and previous root
	path := bt.FindPathToRoot(commitRound)
	// new committed block also certifies the changes from pending rounds
	for _, cb := range path {
		commitNode.data.Changed = append(commitNode.data.Changed, cb.Changed...)
	}
	commitNode.data.CommitQc = commitQc
	// prune the chain, the committed block becomes new root of the chain
	blocksToPrune, err := bt.findBlocksToPrune(commitRound)
	if err != nil {
		return nil, fmt.Errorf("failed to commit block %v, error %w", commitRound, err)
	}
	dbTx, err := bt.blocksDB.StartTx()
	if err != nil {
		return nil, fmt.Errorf("cannot persist block data, %w", err)
	}
	// delete blocks til new root and set establish new root
	for _, round := range blocksToPrune {
		delete(bt.roundToNode, round)
		_ = dbTx.Delete(util.Uint64ToBytes(round))
	}
	// update the new root with commit QC info
	if err = dbTx.Write(util.Uint64ToBytes(commitRound), commitNode.data); err != nil {
		_ = dbTx.Rollback()
		return nil, fmt.Errorf("commit of round %v failed, persist changes failed, %w", commitRound, err)
	}
	// commit changes
	if err = dbTx.Commit(); err != nil {
		return nil, fmt.Errorf("peristing changes to blocks db failed, %w", err)
	}
	bt.root = commitNode
	return commitNode.data, nil
}
