package partition

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"math/rand"

	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/peer"
)

const defaultHandshakeNodes = 3
const defaultNofRootNodes = 2

func randomNodeSelector(nodes peer.IDSlice, upToNodes int) (peer.IDSlice, error) {
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
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, fmt.Errorf("cbor encoder init failed: %w", err)
	}
	ucBytes, err := enc.Marshal(luc)
	if err != nil {
		return nil, fmt.Errorf("UC serialization failed: %w", err)
	}
	hash := sha256.Sum256(ucBytes)
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
