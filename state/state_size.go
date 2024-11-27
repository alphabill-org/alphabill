package state

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
)

type stateSize struct {
	size uint64
}

func (ss *stateSize) Traverse(n *avl.Node[types.UnitID, *Unit]) error {
	if n == nil {
		return nil
	}

	// TODO: reimplement this
	// hackish way to get Data size - we implement Hash interface and
	// let the Data to write itself into the "hasher"
	//n.Value().Data().Write(ss)

	//if err := ss.Traverse(n.Left()); err != nil {
	//	return err
	//}
	//if err := ss.Traverse(n.Right()); err != nil {
	//	return err
	//}
	return nil
}

// Write is a method of the Hash interface, it adds the length of the
// data argument into the total size
func (ss *stateSize) Write(_ any) {
	//ss.size += uint64(len(data))
	// TODO: reimplement this
}

// rest of the Hash interface implementation

func (ss *stateSize) WriteRaw([]byte)      {}
func (ss *stateSize) Sum() ([]byte, error) { return nil, nil }
func (ss *stateSize) Reset()               {}
func (ss *stateSize) Size() int            { return 0 }
