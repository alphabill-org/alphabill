package state

import (
	"fmt"
	"hash"
)

// addToHasher calculates the hash of the node. It also resets the hasher while doing so.
// H(ID, H(StateHash, H(ID, Bearer, UnitData)), self.SummaryValue, leftChild.hash, leftChild.SummaryValue, rightChild.hash, rightChild.summaryValue)
func (n *Node) addToHasher(hasher hash.Hash) {
	leftHash := make([]byte, hasher.Size())
	rightHash := make([]byte, hasher.Size())
	var left = n.Children[0]
	var right = n.Children[1]
	if left != nil {
		leftHash = left.Hash
	}
	if right != nil {
		rightHash = right.Hash
	}

	idBytes := n.ID.Bytes32()

	// Sub hash H(ID, Bearer, UnitData)
	hasher.Reset()
	hasher.Write(idBytes[:])
	hasher.Write(n.Content.Bearer)
	n.Content.Data.AddToHasher(hasher)
	hashSub1 := hasher.Sum(nil)

	// Sub hash H(StateHash, subHash1)
	hasher.Reset()
	hasher.Write(n.Content.StateHash)
	hasher.Write(hashSub1)
	hashSub2 := hasher.Sum(nil)
	// Main hash
	hasher.Reset()
	hasher.Write(idBytes[:])
	hasher.Write(hashSub2)
	n.Content.Data.Value().AddToHasher(hasher)

	hasher.Write(leftHash)
	if left != nil {
		left.SummaryValue.AddToHasher(hasher)
	}
	hasher.Write(rightHash)
	if right != nil {
		right.SummaryValue.AddToHasher(hasher)
	}
}

func (n *Node) LeftChildSummary() SummaryValue {
	if n.Children[0] != nil {
		return n.Children[0].SummaryValue
	}
	return nil
}

func (n *Node) LeftChildHash() []byte {
	if n.Children[0] != nil {
		return n.Children[0].Hash
	}
	return make([]byte, 32)
}

func (n *Node) RightChildSummary() SummaryValue {
	if n.Children[1] != nil {
		return n.Children[1].SummaryValue
	}
	return nil
}

func (n *Node) RightChildHash() []byte {
	if n.Children[1] != nil {
		return n.Children[1].Hash
	}
	return make([]byte, 32)
}

// String returns a string representation of the node
func (n *Node) String() string {
	m := fmt.Sprintf("ID=%v", n.ID.Uint64())
	if n.recompute {
		m = m + "*"
	}

	m = m + fmt.Sprintf(" balance=%d", n.balance)

	if n.Content != nil {
		m = m + fmt.Sprintf(" value=%v, total=%v, bearer=%X, stateHash=%X,",
			n.Content.Data, n.SummaryValue, n.Content.Bearer, n.Content.StateHash)
	}

	if n.Hash != nil {
		m = m + fmt.Sprintf("hash=%X", n.Hash)
	}

	return m
}
