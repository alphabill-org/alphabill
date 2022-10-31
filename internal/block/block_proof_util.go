package block

import (
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/omt"
)

// ToProtobuf utility function that converts []*mt.PathItem to []*MerklePathItem
func ToProtobuf(srcPathItmes []*mt.PathItem) []*MerklePathItem {
	dstPathItems := make([]*MerklePathItem, len(srcPathItmes))
	for i, srcPathItem := range srcPathItmes {
		dstPathItems[i] = &MerklePathItem{
			DirectionLeft: srcPathItem.DirectionLeft,
			PathItem:      srcPathItem.Hash,
		}
	}
	return dstPathItems
}

// FromProtobuf utility function that converts []*MerklePathItem to []*mt.PathItem
func FromProtobuf(chain []*MerklePathItem) []*mt.PathItem {
	dst := make([]*mt.PathItem, len(chain))
	for i, src := range chain {
		dst[i] = &mt.PathItem{
			Hash:          src.PathItem,
			DirectionLeft: src.DirectionLeft,
		}
	}
	return dst
}

// ToProtobufHashChain utility function that converts []omt.Data to []ChainItem
func ToProtobufHashChain(chain []*omt.Data) []*ChainItem {
	r := make([]*ChainItem, len(chain))
	for i, c := range chain {
		r[i] = &ChainItem{Val: c.Val, Hash: c.Hash}
	}
	return r
}

// FromProtobufHashChain utility function that converts []ChainItem []omt.Data
func FromProtobufHashChain(chain []*ChainItem) []*omt.Data {
	r := make([]*omt.Data, len(chain))
	for i, c := range chain {
		r[i] = &omt.Data{Val: c.Val, Hash: c.Hash}
	}
	return r
}
