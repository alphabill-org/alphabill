package partition

import (
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const defaultNofRootNodes = 2

func rootNodesSelector(luc *types.UnicityCertificate, nodes peer.IDSlice, upToNodes int) (peer.IDSlice, error) {
	if luc == nil {
		return nil, fmt.Errorf("UC is nil")
	}
	rootNodes := len(nodes)
	if rootNodes < 1 {
		return nil, fmt.Errorf("root node list is empty")
	}
	// optimization for wanting more nodes than in the root node list - there is not enough to choose from
	if upToNodes >= rootNodes {
		return nodes, nil
	}
	chosen := make(peer.IDSlice, 0, upToNodes)
	hash := sha256.Sum256(luc.Bytes())
	index := int(big.NewInt(0).Mod(big.NewInt(0).SetBytes(hash[:]), big.NewInt(int64(rootNodes))).Int64())
	// choose upToNodes from index
	idx := index
	for {
		chosen = append(chosen, nodes[idx])
		idx++
		// wrap around and choose from node 0
		if idx >= rootNodes {
			idx = 0
		}
		// break loop if either again at start index or enough validators have been found
		if idx == index || len(chosen) == upToNodes {
			break
		}
	}
	return chosen, nil
}
