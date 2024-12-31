package trustbase

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func NewTrustBase(t *testing.T, verifiers ...crypto.Verifier) types.RootTrustBase {
	var nodes []*types.NodeInfo
	for _, v := range verifiers {
		nodes = append(nodes, types.NewNodeInfoFromVerifier("test", 1, v))
	}
	tb, err := types.NewTrustBaseGenesis(nodes, []byte{1})
	require.NoError(t, err)
	return tb
}

func NewTrustBaseFromVerifiers(t *testing.T, verifiers map[string]crypto.Verifier) types.RootTrustBase {
	var nodes []*types.NodeInfo
	for nodeID, v := range verifiers {
		nodes = append(nodes, types.NewNodeInfoFromVerifier(nodeID, 1, v))
	}
	tb, err := types.NewTrustBaseGenesis(nodes, []byte{1})
	require.NoError(t, err)
	return tb
}
