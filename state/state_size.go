package state

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
)

type stateSize struct {
	size uint64
	err  error
}

func (ss *stateSize) Traverse(n *avl.Node[types.UnitID, *Unit]) {
	if n == nil || ss.err != nil {
		return
	}

	// hackish way to get Data size - we implement Hash interface and
	// let the Data to write itself into the "hasher"
	ss.err = n.Value().Data().Write(ss)

	ss.Traverse(n.Left())
	ss.Traverse(n.Right())
}

// Write is a method of the Hash interface, it adds the length of the
// data argument into the total size
func (ss *stateSize) Write(data []byte) (int, error) {
	ss.size += uint64(len(data))
	return 0, nil
}

// rest of the Hash interface implementation

func (ss *stateSize) Sum(_ []byte) []byte { return nil }
func (ss *stateSize) Reset()              {}
func (ss *stateSize) Size() int           { return 0 }
func (ss *stateSize) BlockSize() int      { return 0 }
