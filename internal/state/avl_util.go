// Copyright (c) 2017, Benjamin Scher Purcell. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package state

func put(key uint64, content *BillContent, p *Node, qp **Node) bool {
	q := *qp
	if q == nil {
		*qp = &Node{BillID: key, Bill: content, Parent: p, recompute: true}
		return true
	}

	q.recompute = true
	c := compare(key, q.BillID)
	if c == 0 {
		q.BillID = key
		q.Bill = content
		return false
	}

	if c < 0 {
		c = -1
	} else {
		c = 1
	}
	a := (c + 1) / 2
	var fix bool
	fix = put(key, content, q, &q.Children[a])
	if fix {
		return putFix(int8(c), qp)
	}
	return false
}

func getNode(s *State, key uint64) (*Node, bool) {
	n := s.root
	for n != nil {
		cmp := compare(key, n.BillID)
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

func removeMin(qp **Node, minKey *uint64, minContent **BillContent) bool {
	q := *qp
	if q.Children[0] == nil {
		*minKey = q.BillID
		*minContent = q.Bill
		if q.Children[1] != nil {
			q.Children[1].Parent = q.Parent
		}
		*qp = q.Children[1]
		return true
	}
	fix := removeMin(&q.Children[0], minKey, minContent)
	if fix {
		return removeFix(1, qp)
	}
	return false
}

func putFix(c int8, t **Node) bool {
	s := *t
	if s.balance == 0 {
		s.balance = c
		return true
	}

	if s.balance == -c {
		s.balance = 0
		return false
	}

	if s.Children[(c+1)/2].balance == c {
		s = singlerot(c, s)
	} else {
		s = doublerot(c, s)
	}
	*t = s
	return false
}

func remove(key uint64, qp **Node) bool {
	// TODO recalculation!!
	q := *qp
	if q == nil {
		return false
	}

	c := compare(key, q.BillID)
	if c == 0 {
		if q.Children[1] == nil {
			if q.Children[0] != nil {
				q.Children[0].Parent = q.Parent
			}
			*qp = q.Children[0]
			return true
		}
		fix := removeMin(&q.Children[1], &q.BillID, &q.Bill)
		if fix {
			return removeFix(-1, qp)
		}
		return false
	}

	if c < 0 {
		c = -1
	} else {
		c = 1
	}
	a := (c + 1) / 2
	fix := remove(key, &q.Children[a])
	if fix {
		return removeFix(int8(-c), qp)
	}
	return false
}

func removeFix(c int8, t **Node) bool {
	s := *t
	if s.balance == 0 {
		s.balance = c
		return false
	}

	if s.balance == -c {
		s.balance = 0
		return true
	}

	a := (c + 1) / 2
	if s.Children[a].balance == 0 {
		s = rotate(c, s)
		s.balance = -c
		*t = s
		return false
	}

	if s.Children[a].balance == c {
		s = singlerot(c, s)
	} else {
		s = doublerot(c, s)
	}
	*t = s
	return true
}

func singlerot(c int8, s *Node) *Node {
	s.balance = 0
	s = rotate(c, s)
	s.balance = 0
	return s
}

func doublerot(c int8, s *Node) *Node {
	a := (c + 1) / 2
	r := s.Children[a]
	s.Children[a] = rotate(-c, s.Children[a])
	p := rotate(c, s)

	switch {
	default:
		s.balance = 0
		r.balance = 0
	case p.balance == c:
		s.balance = -c
		r.balance = 0
		s.recompute = true
	case p.balance == -c:
		s.balance = 0
		r.balance = c
		s.recompute = true
	}

	p.balance = 0
	return p
}

func rotate(c int8, s *Node) *Node {
	a := (c + 1) / 2
	r := s.Children[a]
	s.Children[a] = r.Children[a^1]
	if s.Children[a] != nil {
		s.Children[a].Parent = s
	}
	r.Children[a^1] = s
	r.Parent = s.Parent
	s.Parent = r
	return r
}

func compare(a, b uint64) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}

func output(node *Node, prefix string, isTail bool, str *string) {
	if node.Children[1] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "│   "
		} else {
			newPrefix += "    "
		}
		output(node.Children[1], newPrefix, false, str)
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
		output(node.Children[0], newPrefix, true, str)
	}
}
