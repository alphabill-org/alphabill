package proof

import (
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/omt"
)

// ToProtobuf utility function that converts []mt.PathItem to proof.BlockMerkleProof
func ToProtobuf(srcPathItmes []*mt.PathItem) *BlockMerkleProof {
	dstPathItems := make([]*MerklePathItem, len(srcPathItmes))
	for i, srcPathItem := range srcPathItmes {
		dstPathItems[i] = &MerklePathItem{
			DirectionLeft: srcPathItem.DirectionLeft,
			PathItem:      srcPathItem.Hash,
		}
	}
	return &BlockMerkleProof{
		PathItems: dstPathItems,
	}
}

// FromProtobuf utility function that converts BlockMerkleProof to []mt.PathItem
func FromProtobuf(proof *BlockMerkleProof) []*mt.PathItem {
	dstPathItems := make([]*mt.PathItem, len(proof.PathItems))
	for i, srcPathItem := range proof.PathItems {
		dstPathItems[i] = &mt.PathItem{
			Hash:          srcPathItem.PathItem,
			DirectionLeft: srcPathItem.DirectionLeft,
		}
	}
	return dstPathItems
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
