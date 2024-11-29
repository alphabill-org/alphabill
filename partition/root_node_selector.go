package partition

import (
	gocrypto "crypto"
	"fmt"
	"math/big"
	"math/rand"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const defaultHandshakeNodes = 3
const defaultNofRootNodes = 2

func randomNodeSelector(nodes peer.IDSlice, upToNodes int) (peer.IDSlice, error) {
	nodeCnt := len(nodes)
	if nodeCnt < 1 {
		return nil, fmt.Errorf("node list is empty")
	}
	if upToNodes == 0 {
		return nil, fmt.Errorf("invalid parameter, number of nodes to select is 0")
	}
	// optimization for wanting more nodes than in the node list - there is not enough to choose from
	if upToNodes >= nodeCnt {
		return nodes, nil
	}
	rNodes := make(peer.IDSlice, len(nodes))
	// make a copy of available nodes
	copy(rNodes, nodes)
	// randomize
	rand.Shuffle(len(rNodes), func(i, j int) {
		rNodes[i], rNodes[j] = rNodes[j], rNodes[i]
	})
	return rNodes[:upToNodes], nil
}

func rootNodesSelector(luc *types.UnicityCertificate, nodes peer.IDSlice, upToNodes int) (peer.IDSlice, error) {
	if luc == nil {
		return nil, fmt.Errorf("UC is nil")
	}
	nodeCnt := len(nodes)
	if nodeCnt < 1 {
		return nil, fmt.Errorf("root node list is empty")
	}
	if upToNodes == 0 {
		return nil, fmt.Errorf("invalid parameter, number of nodes to select is 0")
	}
	// optimization for wanting more nodes than in the root node list - there is not enough to choose from
	if upToNodes >= nodeCnt {
		return nodes, nil
	}
	chosen := make(peer.IDSlice, 0, upToNodes)
	lucHash, err := luc.Hash(gocrypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to hash UC: %w", err)
	}
	index := int(big.NewInt(0).Mod(big.NewInt(0).SetBytes(lucHash), big.NewInt(int64(nodeCnt))).Int64())
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
