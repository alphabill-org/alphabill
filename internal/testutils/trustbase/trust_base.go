package trustbase

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/stretchr/testify/require"
)

type AlwaysValidTrustBase struct{}

func NewTrustBase(t *testing.T, verifiers ...crypto.Verifier) types.RootTrustBase {
	var nodes []*types.NodeInfo
	for _, v := range verifiers {
		nodes = append(nodes, NewNodeInfoFromVerifier("test", 1, v))
	}
	tb, err := types.NewTrustBaseGenesis(nodes, []byte{1})
	require.NoError(t, err)
	return tb
}

func NewAlwaysValidTrustBase(t *testing.T) types.RootTrustBase {
	return &AlwaysValidTrustBase{}
}

func NewTrustBaseFromVerifiers(t *testing.T, verifiers map[string]crypto.Verifier) types.RootTrustBase {
	var nodes []*types.NodeInfo
	for nodeID, v := range verifiers {
		nodes = append(nodes, NewNodeInfoFromVerifier(nodeID, 1, v))
	}
	tb, err := types.NewTrustBaseGenesis(nodes, []byte{1})
	require.NoError(t, err)
	return tb
}

func NewNodeInfoFromVerifier(nodeID string, stake uint64, sigVerifier crypto.Verifier) *types.NodeInfo {
	sigKey, err := sigVerifier.MarshalPublicKey()
	if err != nil {
		panic("failed to marshal abcrypto.Verifier to public key bytes")
	}
	return &types.NodeInfo{
		NodeID:      nodeID,
		SigKey:      sigKey,
		Stake:       stake,
	}
}

func (a AlwaysValidTrustBase) VerifyQuorumSignatures(data []byte, signatures map[string]hex.Bytes) (error, []error) {
	return nil, nil
}

func (a AlwaysValidTrustBase) VerifySignature(data []byte, sig []byte, nodeID string) (uint64, error) {
	return 1, nil
}

func (a AlwaysValidTrustBase) GetQuorumThreshold() uint64 {
	return 1
}

func (a AlwaysValidTrustBase) GetMaxFaultyNodes() uint64 {
	return 0
}

func (a AlwaysValidTrustBase) GetRootNodes() []*types.NodeInfo {
	return nil
}
