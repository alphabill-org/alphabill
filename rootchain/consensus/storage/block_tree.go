package storage

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	abdrc "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
	// tree node creates a tree of consecutive blocks
	node struct {
		data  *ExecutedBlock
		child []*node // child nodes
	}

	BlockTree struct {
		root        *node
		roundToNode map[uint64]*node
		highQc      *abdrc.QuorumCert
		blocksDB    keyvaluedb.KeyValueDB
	}
)

var (
	ErrCommitFailed = errors.New("commit failed")
)

func newNode(b *ExecutedBlock) *node {
	return &node{data: b, child: make([]*node, 0, 2)}
}

func (l *node) addChild(child *node) {
	l.child = append(l.child, child)
}

func (l *node) removeChild(child *node) {
	for i, n := range l.child {
		if n == child {
			l.child = append(l.child[:i], l.child[i+1:]...)
		}
	}
}

func blockStoreGenesisInit(genesisBlock *ExecutedBlock, blocks keyvaluedb.KeyValueDB) error {
	if err := blocks.Write(blockKey(genesisBlock.GetRound()), genesisBlock); err != nil {
		return fmt.Errorf("genesis block write failed, %w", err)
	}
	return nil
}

func NewBlockTreeFromRecovery(block *ExecutedBlock, bDB keyvaluedb.KeyValueDB) (*BlockTree, error) {
	rootNode := newNode(block)
	hQC := block.CommitQc
	treeNodes := map[uint64]*node{rootNode.data.GetRound(): rootNode}
	if err := bDB.Write(blockKey(block.GetRound()), block); err != nil {
		return nil, fmt.Errorf("block write failed, %w", err)
	}
	return &BlockTree{
		roundToNode: treeNodes,
		root:        rootNode,
		highQc:      hQC,
		blocksDB:    bDB,
	}, nil
}

func readBlocksFromDB(bDB keyvaluedb.KeyValueDB) (_ []*ExecutedBlock, err error) {
	itr := bDB.Find([]byte(blockPrefix))
	defer func() { err = errors.Join(err, itr.Close()) }()
	var blocks []*ExecutedBlock
	for ; itr.Valid() && strings.HasPrefix(string(itr.Key()), blockPrefix); itr.Next() {
		var b ExecutedBlock
		if err = itr.Value(&b); err != nil {
			return nil, fmt.Errorf("read block %v from db failed, %w", itr.Key(), err)
		}
		blocks = append(blocks, &b)
	}
	return blocks, nil
}

func NewBlockTree(bDB keyvaluedb.KeyValueDB) (*BlockTree, error) {
	if bDB == nil {
		return nil, fmt.Errorf("block tree init failed, database is nil")
	}
	var hQC *abdrc.QuorumCert
	blocks, err := readBlocksFromDB(bDB)
	if err != nil {
		return nil, fmt.Errorf("root DB read error: %w", err)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("block tree init failed to recover latest committed block")
	}
	// sort by round number
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].GetRound() < blocks[j].GetRound()
	})
	// find root
	rootIdx := -1
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].CommitQc != nil {
			rootIdx = i
			break
		}
	}
	if rootIdx == -1 {
		return nil, fmt.Errorf("root block not found")
	}
	rootNode := newNode(blocks[rootIdx])
	hQC = rootNode.data.CommitQc
	treeNodes := map[uint64]*node{rootNode.data.GetRound(): rootNode}
	for i := rootIdx + 1; i < len(blocks); i++ {
		block := blocks[i]
		// if parent round does not exist then reject, parent must be recovered
		parent, found := treeNodes[block.GetParentRound()]
		if !found {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block %v not found", block.GetRound(),
				block.GetParentRound())
		}
		// append block and add a child to parent
		n := newNode(block)
		treeNodes[block.BlockData.Round] = n
		parent.addChild(n)
		if n.data.BlockData.Qc.GetRound() > hQC.GetRound() {
			hQC = n.data.BlockData.Qc
		}
	}
	// clear all blocks until new root if any
	for _, b := range blocks {
		if b.GetRound() < rootNode.data.GetRound() {
			if err = bDB.Delete(blockKey(b.GetRound())); err != nil {
				return nil, fmt.Errorf("failed to clean old round from DB")
			}
		}
	}
	return &BlockTree{
		roundToNode: treeNodes,
		root:        rootNode,
		highQc:      hQC,
		blocksDB:    bDB,
	}, nil
}

func (bt *BlockTree) InsertQc(qc *abdrc.QuorumCert) error {
	// find block, if it does not exist, return error we need to recover missing info
	b, err := bt.FindBlock(qc.GetRound())
	if err != nil {
		return fmt.Errorf("block tree add qc failed, %w", err)
	}
	if !bytes.Equal(b.RootHash, qc.VoteInfo.CurrentRootHash) {
		return fmt.Errorf("block tree add qc failed, qc state hash is different from local computed state hash")
	}
	b.Qc = qc
	// persist changes
	if err = bt.blocksDB.Write(blockKey(b.GetRound()), b); err != nil {
		return fmt.Errorf("failed to persist block for round %v, %w", b.BlockData.Round, err)
	}
	bt.highQc = qc
	return nil
}

func (bt *BlockTree) HighQc() *abdrc.QuorumCert {
	return bt.highQc
}

// Add adds new leaf to the block tree
func (bt *BlockTree) Add(block *ExecutedBlock) error {
	// every round can exist only once
	// reject a block if this round has already been added
	if _, found := bt.roundToNode[block.GetRound()]; found {
		return fmt.Errorf("error block for round %v already exists", block.BlockData.Round)
	}
	// if parent round does not exist then reject, parent must be recovered
	parent, found := bt.roundToNode[block.GetParentRound()]
	if !found {
		return fmt.Errorf("error cannot add block for round %v, parent block %v not found", block.GetRound(),
			block.GetParentRound())
	}
	// append block and add a child to parent
	n := newNode(block)
	parent.addChild(n)
	bt.roundToNode[block.GetRound()] = n
	// persist block
	return bt.blocksDB.Write(blockKey(block.GetRound()), n.data)
}

// RemoveLeaf removes leaf node if it is not root node
func (bt *BlockTree) RemoveLeaf(round uint64) error {
	// root cannot be removed
	if bt.root.data.GetRound() == round {
		return fmt.Errorf("error root cannot be removed")
	}
	n, found := bt.roundToNode[round]
	if !found {
		// this is ok if we do not have the node, on TC remove might be triggered twice
		return nil
	}
	if len(n.child) > 0 {
		return fmt.Errorf("error round %v is not leaf node", round)
	}
	parent, found := bt.roundToNode[n.data.GetParentRound()]
	if !found {
		return fmt.Errorf("error parent block %v not found", n.data.GetParentRound())
	}
	delete(bt.roundToNode, round)
	parent.removeChild(n)
	return bt.blocksDB.Delete(blockKey(round))
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
	if !f {
		return nil
	}
	// if the node is root
	if n == bt.root {
		return []*ExecutedBlock{}
	}
	// find path
	path := make([]*ExecutedBlock, 0, 2)
	for {
		if parent, found := bt.roundToNode[n.data.GetParentRound()]; found {
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
	// start from root children
	var blocksToCheck []*node
	blocksToCheck = append(blocksToCheck, bt.root.child...)
	for len(blocksToCheck) > 0 {
		var n *node
		// pop last node from blocks to check
		n, blocksToCheck = blocksToCheck[len(blocksToCheck)-1], blocksToCheck[:len(blocksToCheck)-1]
		// append it's child nodes to check for root
		blocksToCheck = append(blocksToCheck, n.child...)
		// if this node was not the new root then append to pruned blocks
		blocks = append(blocks, n.data)
	}
	return blocks
}

// findBlocksToPrune will return all blocks that can be removed from previous root to new root
// - In a normal case there is only one block, the previous root that will be pruned
// - In case of timeouts there can be more than one, the blocks in between old root and new
// are committed by the same QC.
/*
For example:
	B5--> B6--> B7
	       ╰--> B8--> B9
a call findBlocksToPrune(newRootRound = 9) results in a rounds, 5,6,7,8 all pruned new root node is 9
a call findBlocksToPrune(newRootRound = 8) results in a rounds, 5,6,7 pruned new root node is 8 and tree B8->B9
*/
func (bt *BlockTree) findBlocksToPrune(newRootRound uint64) ([]uint64, error) {
	prunedBlocks := make([]uint64, 0, 2)
	// nothing to be pruned
	if newRootRound == bt.root.data.GetRound() {
		return prunedBlocks, nil
	}
	blocksToCheck := []*node{bt.root}
	newRootFound := false
	for len(blocksToCheck) > 0 {
		var n *node
		// pop last node from blocks to check
		n, blocksToCheck = blocksToCheck[len(blocksToCheck)-1], blocksToCheck[:len(blocksToCheck)-1]
		for _, child := range n.child {
			if child.data.GetRound() == newRootRound {
				newRootFound = true
				continue
			}
			// append new node to check for root
			blocksToCheck = append(blocksToCheck, child)
		}
		// if this node was not the new root then append to pruned blocks
		prunedBlocks = append(prunedBlocks, n.data.GetRound())
	}
	if !newRootFound {
		return nil, fmt.Errorf("new root round %v not found", newRootRound)
	}
	return prunedBlocks, nil
}

func (bt *BlockTree) FindBlock(round uint64) (*ExecutedBlock, error) {
	if b, found := bt.roundToNode[round]; found {
		return b.data, nil
	}
	return nil, fmt.Errorf("block for round %v not found", round)
}

// Commit commits block for round and prunes all preceding blocks from the tree,
// the committed block becomes the new root of the tree
func (bt *BlockTree) Commit(commitQc *abdrc.QuorumCert) (execBlock *ExecutedBlock, err error) {
	// Add qc to pending state (needed for recovery)
	commitRound := commitQc.GetParentRound()
	commitNode, found := bt.roundToNode[commitRound]
	if !found {
		return nil, errors.Join(ErrCommitFailed, fmt.Errorf("block for round %v block not found", commitRound))
	}
	// Find if there are uncommitted nodes between new root and previous root
	path := bt.FindPathToRoot(commitRound)
	// new committed block also certifies the changes from pending rounds
	for _, cb := range path {
		commitNode.data.Changed = append(commitNode.data.Changed, cb.Changed...)
	}
	// prune the chain, the committed block becomes new root of the chain
	blocksToPrune, err := bt.findBlocksToPrune(commitRound)
	if err != nil {
		return nil, fmt.Errorf("failed to commit block %d: %w", commitRound, err)
	}
	dbTx, err := bt.blocksDB.StartTx()
	if err != nil {
		return nil, fmt.Errorf("cannot persist block data: %w", err)
	}
	// delete blocks til new root and set establish new root
	for _, round := range blocksToPrune {
		delete(bt.roundToNode, round)
		if txErr := dbTx.Delete(blockKey(round)); txErr != nil {
			err = errors.Join(err, txErr)
		}
	}
	// update the new root with commit QC info
	commitNode.data.CommitQc = commitQc
	if err = dbTx.Write(blockKey(commitRound), commitNode.data); err != nil {
		if rollbackErr := dbTx.Rollback(); rollbackErr != nil {
			// append also the rollback error for reference
			err = errors.Join(err, rollbackErr)
		}
		return nil, fmt.Errorf("committing round %d, persist changes failed: %w", commitRound, err)
	}
	// commit changes
	if err = dbTx.Commit(); err != nil {
		return nil, fmt.Errorf("persisting changes to blocks db failed: %w", err)
	}
	bt.root = commitNode
	return commitNode.data, err
}
