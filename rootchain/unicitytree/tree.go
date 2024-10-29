package unicitytree

import (
	"cmp"
	"crypto"
	"errors"
	"fmt"
	"slices"

	"github.com/alphabill-org/alphabill-go-base/tree/imt"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	UnicityTree struct {
		imt     *imt.Tree
		sdrhMap map[types.PartitionID][]byte
	}
)

// New creates a new unicity tree with given input records.
func New(hashAlgorithm crypto.Hash, data []*types.UnicityTreeData) (*UnicityTree, error) {
	// sort by index - partition id
	slices.SortFunc(data, func(a, b *types.UnicityTreeData) int {
		return cmp.Compare(a.PartitionIdentifier, b.PartitionIdentifier)
	})
	sdMap := make(map[types.PartitionID][]byte)
	leaves := make([]imt.LeafData, len(data))
	for i, d := range data {
		leaves[i] = d
		sdMap[d.PartitionIdentifier] = d.PartitionDescriptionHash
	}
	t, err := imt.New(hashAlgorithm, leaves)
	if err != nil {
		return nil, fmt.Errorf("index tree construction failed: %w", err)
	}
	return &UnicityTree{
		imt:     t,
		sdrhMap: sdMap,
	}, nil
}

func (u *UnicityTree) GetRootHash() []byte {
	return u.imt.GetRootHash()
}

// GetCertificate returns an unicity tree certificate for given partition identifier.
func (u *UnicityTree) GetCertificate(sysID types.PartitionID) (*types.UnicityTreeCertificate, error) {
	if sysID == 0 {
		return nil, errors.New("partition ID is unassigned")
	}
	sdrh, found := u.sdrhMap[sysID]
	if !found {
		return nil, fmt.Errorf("certificate for partition id %s not found", sysID)
	}
	path, err := u.imt.GetMerklePath(sysID.Bytes())
	if err != nil {
		return nil, err
	}
	return &types.UnicityTreeCertificate{
		PartitionIdentifier:      sysID,
		PartitionDescriptionHash: sdrh,
		HashSteps:                path[1:], // drop redundant first hash step; path is guaranteed to have size > 0
	}, nil
}
