package storage

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	abdrc "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
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
		highQc      *abdrc.QuorumCert
		blocksDB    keyvaluedb.KeyValueDB
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

func blockStoreGenesisInit(genesisBlock *ExecutedBlock, blocks keyvaluedb.KeyValueDB) error {
	if err := blocks.Write(blockKey(genesisBlock.GetRound()), genesisBlock); err != nil {
		return fmt.Errorf("genesis block write failed, %w", err)
	}
	return nil
}

func NewBlockTreeFromRecovery(cBlock *ExecutedBlock, nodes []*ExecutedBlock, bDB keyvaluedb.KeyValueDB) (*BlockTree, error) {
	rootNode := newNode(cBlock)
	hQC := rootNode.data.Qc
	// make sure that nodes are sorted by round
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].GetRound() < nodes[j].GetRound()
	})
	treeNodes := map[uint64]*node{rootNode.data.GetRound(): rootNode}
	for _, next := range nodes {
		// if parent round does not exist then reject, parent must be recovered
		parent, found := treeNodes[next.GetParentRound()]
		if !found {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block %v not found", next.GetRound(),
				next.BlockData.Qc.VoteInfo.RoundNumber)
		}
		// append block and add a child to parent
		n := newNode(next)
		treeNodes[next.GetRound()] = n
		if err := parent.addChild(n); err != nil {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block add child error, %w", next.GetRound(), err)
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

func NewBlockTree(bDB keyvaluedb.KeyValueDB) (bTree *BlockTree, err error) {
	if bDB == nil {
		return nil, fmt.Errorf("block tree init failed, database is nil")
	}
	itr := bDB.Find([]byte(blockPrefix))
	defer func() { err = errors.Join(err, itr.Close()) }()
	var blocks []*ExecutedBlock
	var hQC *abdrc.QuorumCert = nil
	for ; itr.Valid() && strings.HasPrefix(string(itr.Key()), blockPrefix); itr.Next() {
		var b ExecutedBlock
		if err = itr.Value(&b); err != nil {
			return nil, fmt.Errorf("read block %v from db failed, %w", itr.Key(), err)
		}
		blocks = append(blocks, &b)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("block tree init failed to recover latest committed block")
	}
	var rootNode *node = nil
	treeNodes := make(map[uint64]*node)
	for i, block := range blocks {
		// last root
		if i == 0 {
			rootNode = newNode(block)
			hQC = rootNode.data.Qc
			treeNodes = map[uint64]*node{block.GetRound(): rootNode}
			continue
		}
		// if parent round does not exist then reject, parent must be recovered
		parent, found := treeNodes[block.GetParentRound()]
		if !found {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block %v not found", block.GetRound(),
				block.GetParentRound())
		}
		// append block and add a child to parent
		n := newNode(block)
		treeNodes[block.BlockData.Round] = n
		if err = parent.addChild(n); err != nil {
			return nil, fmt.Errorf("error cannot add block for round %v, parent block add child error, %w", block.GetRound(), err)
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

func (bt *BlockTree) InsertQc(qc *abdrc.QuorumCert, bockDB keyvaluedb.KeyValueDB) error {
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
	if err = bockDB.Write(blockKey(b.GetRound()), b); err != nil {
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
	if err := parent.addChild(n); err != nil {
		return fmt.Errorf("error cannot add block for round %v, parent block add child error, %w", block.GetRound(), err)
	}
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
	if n.child != nil {
		return fmt.Errorf("error round %v is not child node", round)
	}
	parent, found := bt.roundToNode[n.data.GetParentRound()]
	if !found {
		return fmt.Errorf("error parent block %v not found", n.data.GetParentRound())
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
	for n := bt.root.child; n != nil; n = n.child {
		blocks = append(blocks, n.data)
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
	if newRootRound == bt.root.data.GetRound() {
		return blocksToPrune, nil
	}
	treeNode := bt.root
	newRootFound := false
	for treeNode.child != nil {
		blocksToPrune = append(blocksToPrune, treeNode.data.GetRound())
		// if the child is to become the new root then stop here
		if treeNode.child.data.GetRound() == newRootRound {
			newRootFound = true
			break
		}
		treeNode = treeNode.child
	}
	if !newRootFound {
		return nil, fmt.Errorf("error, new root round %v not found", newRootRound)
	}
	return blocksToPrune, nil
}

func (bt *BlockTree) FindBlock(round uint64) (*ExecutedBlock, error) {
	if b, found := bt.roundToNode[round]; found {
		return b.data, nil
	}
	return nil, fmt.Errorf("block for round %v not found", round)
}

// Commit commits block for round and prunes all preceding blocks from the tree,
// the committed block becomes the new root of the tree
func (bt *BlockTree) Commit(commitQc *abdrc.QuorumCert) (*ExecutedBlock, error) {
	// Add qc to pending state (needed for recovery)
	commitRound := commitQc.GetParentRound()
	commitNode, found := bt.roundToNode[commitRound]
	if !found {
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
		return nil, fmt.Errorf("failed to commit block %d: %w", commitRound, err)
	}
	dbTx, err := bt.blocksDB.StartTx()
	if err != nil {
		return nil, fmt.Errorf("cannot persist block data: %w", err)
	}
	// delete blocks til new root and set establish new root
	for _, round := range blocksToPrune {
		delete(bt.roundToNode, round)
		_ = dbTx.Delete(util.Uint64ToBytes(round))
	}
	// update the new root with commit QC info
	if err = dbTx.Write(blockKey(commitRound), commitNode.data); err != nil {
		_ = dbTx.Rollback()
		return nil, fmt.Errorf("committing round %d, persist changes failed: %w", commitRound, err)
	}
	// commit changes
	if err = dbTx.Commit(); err != nil {
		return nil, fmt.Errorf("persisting changes to blocks db failed: %w", err)
	}
	bt.root = commitNode
	return commitNode.data, nil
}
