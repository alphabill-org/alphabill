package partition

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"math/rand"

	"github.com/alphabill-org/alphabill/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const defaultHandshakeNodes = 3
const defaultNofRootNodes = 2

func randomNodeSelector(nodes peer.IDSlice, upToNodes int) (peer.IDSlice, error) {
	nodeCnt := len(nodes)
	if nodeCnt < 1 {
		return nil, fmt.Errorf("root node list is empty")
	}
	// optimization for wanting more nodes than in the root node list - there is not enough to choose from
	if upToNodes >= nodeCnt {
		return nodes, nil
	}
	chosen := make(peer.IDSlice, 0, upToNodes)
	// choose the requested number of nodes starting from random index
	// #nosec G404
	index := rand.Intn(len(nodes))
	// choose upToNodes from index
	idx := index
	for {
		chosen = append(chosen, nodes[idx])
		idx++
		// wrap around and choose from node 0
		if idx >= nodeCnt {
			idx = 0
		}
		// break loop if either again at start index or enough validators have been found
		if idx == index || len(chosen) == upToNodes {
			break
		}
	}
	return chosen, nil
}

func rootNodesSelector(luc *types.UnicityCertificate, nodes peer.IDSlice, upToNodes int) (peer.IDSlice, error) {
	if luc == nil {
		return nil, fmt.Errorf("UC is nil")
	}
	nodeCnt := len(nodes)
	if nodeCnt < 1 {
		return nil, fmt.Errorf("root node list is empty")
	}
	// optimization for wanting more nodes than in the root node list - there is not enough to choose from
	if upToNodes >= nodeCnt {
		return nodes, nil
	}
	chosen := make(peer.IDSlice, 0, upToNodes)
	hash := sha256.Sum256(luc.Bytes())
	index := int(big.NewInt(0).Mod(big.NewInt(0).SetBytes(hash[:]), big.NewInt(int64(nodeCnt))).Int64())
	// choose upToNodes from index
	idx := index
	for {
		chosen = append(chosen, nodes[idx])
		idx++
		// wrap around and choose from node 0
		if idx >= nodeCnt {
			idx = 0
		}
		// break loop if either again at start index or enough validators have been found
		if idx == index || len(chosen) == upToNodes {
			break
		}
	}
	return chosen, nil
}
