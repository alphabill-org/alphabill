// Copyright (c) 2017, Benjamin Scher Purcell. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package state

func put(key uint64, content *BillContent, p *Node, qp **Node) bool {
	q := *qp
	if q == nil {
		*qp = &Node{ID: key, Bill: content, Parent: p, recompute: true}
		return true
	}

	q.recompute = true
	c := compare(key, q.ID)
	if c == 0 {
		q.ID = key
		q.Bill = content
		return false
	}

	a := (c + 1) / 2
	var fix bool
	fix = put(key, content, q, &q.Children[a])
	if fix {
		return putFix(c, qp)
	}
	return false
}

func getNode(s *Node, key uint64) (*Node, bool) {
	n := s
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

func compare(a, b uint64) int8 {
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
