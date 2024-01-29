package unicitytree

import (
	"cmp"
	"crypto"
	"errors"
	"fmt"
	"slices"

	"github.com/alphabill-org/alphabill/tree/imt"
	"github.com/alphabill-org/alphabill/types"
)

type (
	UnicityTree struct {
		imt     *imt.Tree
		sdrhMap map[types.SystemID][]byte
	}
)

// New creates a new unicity tree with given input records.
func New(hashAlgorithm crypto.Hash, data []*types.UTData) (*UnicityTree, error) {
	// sort by index - system id
	slices.SortFunc(data, func(a, b *types.UTData) int {
		return cmp.Compare(a.SystemIdentifier, b.SystemIdentifier)
	})
	sdMap := make(map[types.SystemID][]byte)
	leaves := make([]imt.LeafData, len(data))
	for i, d := range data {
		leaves[i] = d
		sdMap[d.SystemIdentifier] = d.SystemDescriptionRecordHash
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

// GetCertificate returns an unicity tree certificate for given system identifier.
func (u *UnicityTree) GetCertificate(sysID types.SystemID) (*types.UnicityTreeCertificate, error) {
	if sysID == 0 {
		return nil, errors.New("partition ID is unassigned")
	}
	sdrh, found := u.sdrhMap[sysID]
	if !found {
		return nil, fmt.Errorf("certificate for system id %s not found", sysID)
	}
	path, err := u.imt.GetMerklePath(sysID.Bytes())
	if err != nil {
		return nil, err
	}
	return &types.UnicityTreeCertificate{
		SystemIdentifier:      sysID,
		SystemDescriptionHash: sdrh,
		SiblingHashes:         path,
	}, nil
}
