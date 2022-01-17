package state

import (
	"crypto"
	"fmt"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"github.com/holiman/uint256"
)

const (
	errStrItemDoesntExist      = "item %d does not exist"
	errStrInvalidHashAlgorithm = "invalid hash algorithm"
)

type (
	Predicate []byte

	// UpdateFunction is a function for updating the data of an item. Taken in previous UnitData and returns new UnitData.
	UpdateFunction func(data UnitData) (newData UnitData)

	// UnitData is generic datatype for the tree. Is connected to SummaryValue through the Value function.
	UnitData interface {
		// AddToHasher adds the value of summary value to the hasher
		AddToHasher(hasher hash.Hash)
		// Value returns the SummaryValue of this single UnitData.
		Value() SummaryValue
	}

	// SummaryValue is different from UnitData. It is derived from UnitData with UnitData.Value function.
	SummaryValue interface {
		// AddToHasher adds the value of summary value to the hasher
		AddToHasher(hasher hash.Hash)
		// Concatenate calculates new SummaryValue by concatenating this, left and right.
		Concatenate(left, right SummaryValue) SummaryValue
	}

	Unit struct {
		Bearer    Predicate // The owner predicate of the Item/Node
		Data      UnitData  // The Data part of the Item/Node
		StateHash []byte    // The hash of the transaction and previous transaction. See spec 'Unit Ledger' for details.
	}

	Node struct {
		ID           *uint256.Int // The identifier of the Item/Node
		Content      *Unit        // Content that moves together with Node
		SummaryValue SummaryValue // Calculated SummaryValue of the Item/Node
		Hash         []byte       // The hash of the node inside the tree. See spec 'State Invariants and Parameters' for details.
		Parent       *Node        // The parent node
		Children     [2]*Node     // The children (0 - left, 1 - right)
		recompute    bool         // true if node content (hash or summary value) needs recalculation
		balance      int          // AVL specific balance factor
	}

	Config struct {
		HashAlgorithm     crypto.Hash // Mandatory, hash algorithm used for calculating the tree hash root and the proofs.
		RecordingDisabled bool        // Optional, set to true, to disable keeping track of changes.
		ShardId           []byte      // Optional, the ID of the shard. By default, 0.
	}

	// rmaTree Revertible Merkle AVL Tree. Holds any type of units. Changes can be reverted, tree is balanced in AVL tree manner and Merkle proofs can be generated.
	rmaTree struct {
		hashAlgorithm    crypto.Hash   // Hash algorithm used for calculating the tree hash root and the proofs.
		shardId          []byte        // ID of the shard.
		roundNumber      uint64        // The current round number.
		recordingEnabled bool          // recordingEnabled controls if changes are recorded or not.
		root             *Node         // root is the top node of the tree.
		changes          []interface{} // changes keep track of changes. Only used if recordingEnabled is true.
		// TODO add trust base: https://guardtime.atlassian.net/browse/AB-91
	}

	changeNode struct {
		targetPointer **Node
		oldVal        *Node
	}

	changeBalance struct {
		targetNode *Node
		oldVal     int
	}

	changeContent struct {
		targetNode *Node
		oldVal     *Unit
	}

	changeMinKeyMinVal struct {
		minKey    **uint256.Int
		oldMinKey *uint256.Int
		minVal    **Unit
		oldMinVal *Unit
	}

	changeRecompute struct {
		targetNode *Node
		oldVal     bool
	}

	changeHash struct {
		targetNode *Node
		oldVal     []byte
	}

	changeSummaryValue struct {
		targetNode *Node
		oldVal     SummaryValue
	}
)

// New creates new RMA Tree
func New(config *Config) (*rmaTree, error) {
	if config.HashAlgorithm != crypto.SHA256 && config.HashAlgorithm != crypto.SHA512 {
		return nil, errors.New(errStrInvalidHashAlgorithm)
	}
	return &rmaTree{
		hashAlgorithm:    config.HashAlgorithm,
		recordingEnabled: !config.RecordingDisabled,
		shardId:          config.ShardId,
	}, nil
}

// AddItem adds new element to the state. Id must not exist in the state.
func (tree *rmaTree) AddItem(id *uint256.Int, owner Predicate, data UnitData, stateHash []byte) error {
	exists := tree.exists(id)
	if exists {
		return errors.Errorf("cannot add item that already exists. ID: %d", id)
	}
	tree.set(id, owner, data, stateHash)
	return nil
}

// DeleteItem removes the item from the state
func (tree *rmaTree) DeleteItem(id *uint256.Int) error {
	exists := tree.exists(id)
	if exists {
		return errors.Errorf("deleting item that does not exist. ID %d", id)
	}
	tree.removeNode(id)
	return nil
}

// SetOwner changes the owner of the item, leaves data as is.
func (tree *rmaTree) SetOwner(id *uint256.Int, owner Predicate, stateHash []byte) error {
	return tree.setOwner(id, owner, stateHash)
}

// UpdateData changes the data of the item, leaves owner as is.
func (tree *rmaTree) UpdateData(id *uint256.Int, f UpdateFunction, stateHash []byte) error {
	node, exists := tree.getNode(id)
	if !exists {
		return errors.Errorf(errStrItemDoesntExist, id)
	}
	data := f(node.Content.Data)
	tree.set(id, node.Content.Bearer, data, stateHash)
	return nil
}

// GetRootHash starts computation of the tree and returns the root node hash value.
func (tree *rmaTree) GetRootHash() []byte {
	if tree.root == nil {
		return nil
	}
	tree.recompute(tree.root, tree.hashAlgorithm.New())
	return tree.root.Hash
}

// TotalValue starts computation of the tree and returns the SummaryValue of the root node.
func (tree *rmaTree) TotalValue() SummaryValue {
	if tree.root == nil {
		return nil
	}
	tree.recompute(tree.root, tree.hashAlgorithm.New())
	return tree.root.SummaryValue
}

// Commit commits the changes, making these not revertible.
// Changes done before the Commit cannot be reverted anymore.
// Changes done after the last Commit can be reverted by Revert method.
func (tree *rmaTree) Commit() {
	tree.changes = []interface{}{}
}

// Revert reverts all changes since the last Commit.
func (tree *rmaTree) Revert() {
	for i := len(tree.changes) - 1; i >= 0; i-- {
		change := tree.changes[i]
		switch chg := change.(type) {
		case *changeNode:
			*chg.targetPointer = chg.oldVal
		case *changeBalance:
			chg.targetNode.balance = chg.oldVal
		case *changeContent:
			chg.targetNode.Content = chg.oldVal
		case *changeMinKeyMinVal:
			*chg.minKey = chg.oldMinKey
			*chg.minVal = chg.oldMinVal
		case *changeRecompute:
			chg.targetNode.recompute = chg.oldVal
		case *changeSummaryValue:
			chg.targetNode.SummaryValue = chg.oldVal
		case *changeHash:
			chg.targetNode.Hash = chg.oldVal
		default:
			panic(fmt.Sprintf("invalid type %T", chg))
		}
	}
	tree.Commit()
}

///////// private methods \\\\\\\\\\\\\

func (tree *rmaTree) get(id *uint256.Int) (unit *Unit, err error) {
	node, exists := tree.getNode(id)
	if !exists {
		return nil, errors.Errorf(errStrItemDoesntExist, id)
	}
	return node.Content, nil
}

// Set sets the item bearer and data. It's up to the caller to make sure the UnitData implementation supports the all data implementations inserted into the tree.
func (tree *rmaTree) set(id *uint256.Int, owner Predicate, data UnitData, stateHash []byte) {
	tree.setNode(id, &Unit{
		Bearer:    owner,
		Data:      data,
		StateHash: stateHash})
}

func (tree *rmaTree) setOwner(id *uint256.Int, owner Predicate, stateHash []byte) error {
	node, exists := tree.getNode(id)
	if !exists {
		return errors.Errorf(errStrItemDoesntExist, id)
	}
	tree.set(id, owner, node.Content.Data, stateHash)
	return nil
}

func (tree *rmaTree) setData(id *uint256.Int, data UnitData, stateHash []byte) error {
	node, exists := tree.getNode(id)
	if !exists {
		return errors.Errorf(errStrItemDoesntExist, id)
	}
	tree.set(id, node.Content.Bearer, data, stateHash)
	return nil
}

func (tree *rmaTree) exists(id *uint256.Int) bool {
	_, exists := tree.getNode(id)
	return exists
}

func (tree *rmaTree) recompute(n *Node, hasher hash.Hash) {
	if n.recompute {
		var leftTotalValue SummaryValue
		var rightTotalValue SummaryValue

		var left = n.Children[0]
		var right = n.Children[1]
		if left != nil {
			tree.recompute(left, hasher)
			leftTotalValue = left.SummaryValue
		}
		if right != nil {
			tree.recompute(right, hasher)
			rightTotalValue = right.SummaryValue
		}
		tree.assignSummaryValue(n, n.Content.Data.Value().Concatenate(leftTotalValue, rightTotalValue))

		hasher.Reset()
		n.addToHasher(hasher)
		tree.assignHash(n, hasher.Sum(nil))
		hasher.Reset()
		tree.assignRecompute(n, false)
	}
}

// setNode adds a new node to the tree or replaces existing one.
func (tree *rmaTree) setNode(key *uint256.Int, content *Unit) {
	tree.put(key, content, nil, &tree.root)
}

// put is recursive method, use setNode to add or update a node.
func (tree *rmaTree) put(key *uint256.Int, content *Unit, p *Node, qp **Node) bool {
	q := *qp
	if q == nil {
		n := &Node{ID: key, Content: content, Parent: p, recompute: true}
		tree.assignNode(qp, n)
		return true
	}

	tree.assignRecompute(q, true)
	c := compare(key, q.ID)
	if c == 0 {
		tree.assignContent(q, content)
		return false
	}

	a := (c + 1) / 2
	var fix bool
	fix = tree.put(key, content, q, &q.Children[a])
	if fix {
		return tree.putFix(c, qp)
	}
	return false
}

// getNode returns the node with given id
func (tree *rmaTree) getNode(key *uint256.Int) (*Node, bool) {
	n := tree.root
	for n != nil {
		cmp := compare(key, n.ID)
		switch {
		case cmp == 0:
			return n, true
		case cmp < 0:
			n = n.Children[0]
		case cmp > 0:
			n = n.Children[1]
		}
	}
	return nil, false
}

// removeNode will find the node and remove from the tree. In case node does not exist won't do anything.
func (tree *rmaTree) removeNode(key *uint256.Int) {
	tree.remove(key, &tree.root)
}

// remove is a recursive function, use removeNode for deleting an item.
func (tree *rmaTree) remove(key *uint256.Int, qp **Node) bool {
	q := *qp
	if q == nil {
		return false
	}

	c := compare(key, q.ID)
	if c == 0 {
		if q.Children[1] == nil {
			if q.Children[0] != nil {
				tree.assignRecompute(q.Parent, true)
				tree.assignNode(&q.Children[0].Parent, q.Parent)
			}

			tree.assignNode(qp, q.Children[0])
			return true
		}
		tree.assignRecompute(q, true)
		fix := tree.removeMin(&q.Children[1], &q.ID, &q.Content)
		if fix {
			return tree.removeFix(-1, qp)
		}
		return false
	}
	tree.assignRecompute(q, true)

	if c < 0 {
		c = -1
	} else {
		c = 1
	}
	a := (c + 1) / 2
	fix := tree.remove(key, &q.Children[a])
	if fix {
		return tree.removeFix(-c, qp)
	}
	return false
}

func (tree *rmaTree) removeMin(qp **Node, minKey **uint256.Int, minVal **Unit) bool {
	q := *qp
	if q.Children[0] == nil {
		tree.assignMinKeyMinVal(minKey, q.ID, minVal, q.Content)
		if q.Children[1] != nil {
			tree.assignNode(&q.Children[1].Parent, q.Parent)
		}
		tree.assignNode(qp, q.Children[1])
		return true
	}
	tree.assignRecompute(q, true)
	fix := tree.removeMin(&q.Children[0], minKey, minVal)
	if fix {
		return tree.removeFix(1, qp)
	}
	return false
}

func (tree *rmaTree) removeFix(c int, t **Node) bool {
	s := *t
	if s.balance == 0 {
		tree.assignBalance(s, c)
		return false
	}

	if s.balance == -c {
		tree.assignBalance(s, 0)
		return true
	}

	a := (c + 1) / 2
	if s.Children[a].balance == 0 {
		s = tree.rotate(c, s)
		tree.assignRecompute(s, true)
		tree.assignBalance(s, -c)
		tree.assignNode(t, s)
		return false
	}

	if s.Children[a].balance == c {
		s = tree.singlerot(c, s)
	} else {
		s = tree.doublerot(c, s)
	}
	tree.assignRecompute(s, true)
	tree.assignNode(t, s)
	return true
}

func (tree *rmaTree) putFix(c int, t **Node) bool {
	s := *t
	if s.balance == 0 {
		tree.assignBalance(s, c)
		return true
	}

	if s.balance == -c {
		tree.assignBalance(s, 0)
		return false
	}

	if s.Children[(c+1)/2].balance == c {
		s = tree.singlerot(c, s)
	} else {
		s = tree.doublerot(c, s)
	}
	tree.assignNode(t, s)
	return false
}

func (tree *rmaTree) singlerot(c int, s *Node) *Node {
	tree.assignBalance(s, 0)
	s = tree.rotate(c, s)
	tree.assignBalance(s, 0)
	return s
}

func (tree *rmaTree) doublerot(c int, s *Node) *Node {
	a := (c + 1) / 2
	r := s.Children[a]
	tree.assignRecompute(s.Children[a], true)
	tree.assignNode(&s.Children[a], tree.rotate(-c, s.Children[a]))
	p := tree.rotate(c, s)

	switch {
	default:
		tree.assignBalance(s, 0)
		tree.assignBalance(r, 0)
	case p.balance == c:
		tree.assignBalance(s, -c)
		tree.assignBalance(r, 0)
		tree.assignRecompute(s, true)
	case p.balance == -c:
		tree.assignBalance(s, 0)
		tree.assignBalance(r, c)
		tree.assignRecompute(s, true)
	}

	tree.assignBalance(p, 0)
	//p.balance = 0
	return p
}

func (tree *rmaTree) rotate(c int, s *Node) *Node {
	a := (c + 1) / 2
	r := s.Children[a]
	tree.assignNode(&s.Children[a], r.Children[a^1])
	if s.Children[a] != nil {
		tree.assignNode(&s.Children[a].Parent, s)
	}
	tree.assignNode(&r.Children[a^1], s)
	tree.assignNode(&r.Parent, s.Parent)
	tree.assignNode(&s.Parent, r)
	return r
}

func (tree *rmaTree) assignNode(target **Node, source *Node) {
	if tree.recordingEnabled {
		tree.changes = append(tree.changes, &changeNode{
			targetPointer: target,
			oldVal:        *target,
		})
	}
	*target = source
}

func (tree *rmaTree) assignBalance(target *Node, balance int) {
	if tree.recordingEnabled {
		tree.changes = append(tree.changes, &changeBalance{
			targetNode: target,
			oldVal:     target.balance,
		})
	}
	target.balance = balance
}

func (tree *rmaTree) assignContent(target *Node, content *Unit) {
	if tree.recordingEnabled {
		tree.changes = append(tree.changes, &changeContent{
			targetNode: target,
			oldVal:     target.Content,
		})
	}
	target.Content = content
}

func (tree *rmaTree) assignMinKeyMinVal(minKey **uint256.Int, id *uint256.Int, minVal **Unit, content *Unit) {
	if tree.recordingEnabled {
		tree.changes = append(tree.changes, &changeMinKeyMinVal{
			minKey:    minKey,
			oldMinKey: *minKey,
			minVal:    minVal,
			oldMinVal: *minVal,
		})
	}
	*minKey = id
	*minVal = content
}

func (tree *rmaTree) assignRecompute(target *Node, recompute bool) {
	if tree.recordingEnabled {
		tree.changes = append(tree.changes, &changeRecompute{
			targetNode: target,
			oldVal:     target.recompute,
		})
	}
	target.recompute = recompute
}

func (tree *rmaTree) assignSummaryValue(target *Node, summary SummaryValue) {
	if tree.recordingEnabled {
		tree.changes = append(tree.changes, &changeSummaryValue{
			targetNode: target,
			oldVal:     target.SummaryValue,
		})
	}
	target.SummaryValue = summary
}

func (tree *rmaTree) assignHash(target *Node, hash []byte) {
	if tree.recordingEnabled {
		tree.changes = append(tree.changes, &changeHash{
			targetNode: target,
			oldVal:     target.Hash,
		})
	}
	target.Hash = hash
}

func compare(a, b *uint256.Int) int {
	return a.Cmp(b)
}

// print generates a human-readable presentation of the avlTree.
func (tree *rmaTree) print() string {
	if tree.root == nil {
		return "tree is empty"
	}
	out := ""
	tree.output(tree.root, "", false, &out)
	return out
}

// output is rmaTree inner method for producing debugging
func (tree *rmaTree) output(node *Node, prefix string, isTail bool, str *string) {
	if node.Children[1] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "│   "
		} else {
			newPrefix += "    "
		}
		tree.output(node.Children[1], newPrefix, false, str)
	}
	*str += prefix
	if isTail {
		*str += "└── "
	} else {
		*str += "┌── "
	}
	*str += node.String() + "\n"
	if node.Children[0] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}
		tree.output(node.Children[0], newPrefix, true, str)
	}
}
