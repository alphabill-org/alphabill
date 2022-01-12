// Copyright (c) 2017, Benjamin Scher Purcell. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package state

import (
	"fmt"

	"github.com/holiman/uint256"
)

type (
	avlTree struct {
		// recordingEnabled controls if changes are recorded or not. If set to false, then revertChanges and resetChanges do nothing.
		recordingEnabled bool
		// root is the top node of the tree.
		root *Node
		// changes keep track of changes. Only if recordingEnabled is true.
		changes []change
	}

	changeType byte

	change struct {
		typ           changeType
		targetPointer **Node
		oldVal        *Node
		targetNode    *Node
		oldBalance    int
		oldContent    *Unit
	}
)

const (
	chgNodeAssignment = changeType(iota)
	chgBalanceAssignment
	chgContentAssignment
)

// setNode adds a new node to the tree
func (at *avlTree) setNode(key *uint256.Int, content *Unit) {
	at.put(key, content, nil, &at.root)
}

// put is internal method of avlTree, use setNode to add or update a node.
func (at *avlTree) put(key *uint256.Int, content *Unit, p *Node, qp **Node) bool {
	q := *qp
	if q == nil {
		n := &Node{ID: key, Content: content, Parent: p, recompute: true}
		at.assignNode(qp, n)
		//*qp = &Node{ID: key, Content: content, Parent: p, recompute: true}
		return true
	}

	q.recompute = true
	c := compare(key, q.ID)
	if c == 0 {
		at.assignContent(q, content)
		//q.Content = content
		return false
	}

	a := (c + 1) / 2
	var fix bool
	fix = at.put(key, content, q, &q.Children[a])
	if fix {
		return at.putFix(c, qp)
	}
	return false
}

// getNode returns the node with given id
func (at *avlTree) getNode(key *uint256.Int) (*Node, bool) {
	n := at.root
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

func (at *avlTree) putFix(c int, t **Node) bool {
	s := *t
	if s.balance == 0 {
		at.assignBalance(s, c)
		//s.balance = c
		return true
	}

	if s.balance == -c {
		at.assignBalance(s, 0)
		//s.balance = 0
		return false
	}

	if s.Children[(c+1)/2].balance == c {
		s = at.singlerot(c, s)
	} else {
		s = at.doublerot(c, s)
	}
	at.assignNode(t, s)
	//*t = s
	return false
}

func (at *avlTree) singlerot(c int, s *Node) *Node {
	at.assignBalance(s, 0)
	//s.balance = 0
	s = at.rotate(c, s)
	at.assignBalance(s, 0)
	//s.balance = 0
	return s
}

func (at *avlTree) doublerot(c int, s *Node) *Node {
	a := (c + 1) / 2
	r := s.Children[a]
	at.assignNode(&s.Children[a], at.rotate(-c, s.Children[a]))
	//s.Children[a] = rotate(-c, s.Children[a])
	p := at.rotate(c, s)

	switch {
	default:
		at.assignBalance(s, 0)
		at.assignBalance(r, 0)
		//s.balance = 0
		//r.balance = 0
	case p.balance == c:
		at.assignBalance(s, -c)
		at.assignBalance(r, 0)
		//s.balance = -c
		//r.balance = 0
		s.recompute = true
	case p.balance == -c:
		at.assignBalance(s, 0)
		at.assignBalance(r, c)
		//s.balance = 0
		//r.balance = c
		s.recompute = true
	}

	at.assignBalance(p, 0)
	//p.balance = 0
	return p
}

// c: {+1 ; -1}
func (at *avlTree) rotate(c int, s *Node) *Node {
	a := (c + 1) / 2
	r := s.Children[a]
	at.assignNode(&s.Children[a], r.Children[a^1])
	//s.Children[a] = r.Children[a^1] // 0>1 ; 1>0
	if s.Children[a] != nil {
		at.assignNode(&s.Children[a].Parent, s)
		//s.Children[a].Parent = s
	}
	at.assignNode(&r.Children[a^1], s)
	//r.Children[a^1] = s
	at.assignNode(&r.Parent, s.Parent)
	//r.Parent = s.Parent
	at.assignNode(&s.Parent, r)
	//s.Parent = r
	return r
}

func (at *avlTree) printChanges() {
	println("changes")
	for i := len(at.changes) - 1; i >= 0; i-- {
		chg := at.changes[i]
		switch chg.typ {
		case chgNodeAssignment:
			println(" ", i, "node assignment")
			println("    target", chg.targetPointer, "value(", *chg.targetPointer, ") oldValue", chg.oldVal)
		case chgBalanceAssignment:
			println(" ", i, "balance assignment")
			println("    target", chg.targetNode, "old balance", chg.oldBalance)
		default:
			panic(fmt.Sprintf("invalid type %d", chg.typ))
		}

	}
	println("")
}

func (at *avlTree) assignNode(target **Node, source *Node) {
	if at.recordingEnabled {
		at.changes = append(at.changes, change{
			typ:           chgNodeAssignment,
			targetPointer: target,
			oldVal:        *target,
		})
	}
	//println("setting", target, "value to", source, "was(", *target, ")")
	*target = source
}

func (at *avlTree) assignBalance(target *Node, balance int) {
	if at.recordingEnabled {
		at.changes = append(at.changes, change{
			typ:        chgBalanceAssignment,
			targetNode: target,
			oldBalance: target.balance,
		})
	}
	target.balance = balance
}

func (at *avlTree) assignContent(target *Node, content *Unit) {
	if at.recordingEnabled {
		at.changes = append(at.changes, change{
			typ:        chgContentAssignment,
			targetNode: target,
			oldContent: target.Content,
		})
	}
	target.Content = content
}

func (at *avlTree) resetChanges() {
	at.changes = []change{}
}

func (at *avlTree) revertChanges() {
	for i := len(at.changes) - 1; i >= 0; i-- {
		chg := at.changes[i]
		switch chg.typ {
		case chgNodeAssignment:
			//println("reverting", chg.targetPointer, "value to", chg.oldVal, "was(", *chg.targetPointer, ")")
			*chg.targetPointer = chg.oldVal
		case chgBalanceAssignment:
			//println("reverting", chg.targetNode, "balance to", chg.oldBalance)
			chg.targetNode.balance = chg.oldBalance
		case chgContentAssignment:
			chg.targetNode.Content = chg.oldContent
		default:
			panic(fmt.Sprintf("invalid type %d", chg.typ))
		}
	}
	at.resetChanges()
}

func compare(a, b *uint256.Int) int {
	return a.Cmp(b)
}

// print generates a human-readable presentation of the avlTree.
func (at *avlTree) print() string {
	out := ""
	at.output(at.root, "", false, &out)
	return out
}

// output is avlTree inner method for producing output
func (at *avlTree) output(node *Node, prefix string, isTail bool, str *string) {
	if node.Children[1] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "│   "
		} else {
			newPrefix += "    "
		}
		at.output(node.Children[1], newPrefix, false, str)
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
		at.output(node.Children[0], newPrefix, true, str)
	}
}
