package trustbase

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/stretchr/testify/require"
)

type AlwaysValidTrustBase struct{}

func NewTrustBase(t *testing.T, verifiers ...abcrypto.Verifier) types.RootTrustBase {
	var nodes []*types.NodeInfo
	for _, v := range verifiers {
		pubKeyBytes, err := v.MarshalPublicKey()
		require.NoError(t, err)
		pubKey, err := crypto.UnmarshalSecp256k1PublicKey(pubKeyBytes)
		require.NoError(t, err)
		peerID, err := peer.IDFromPublicKey(pubKey)
		require.NoError(t, err)

		nodes = append(nodes, &types.NodeInfo{
			NodeID: peerID.String(),
			SigKey: pubKeyBytes,
			Stake:  1,
		})
	}
	tb, err := types.NewTrustBaseGenesis(5, nodes)
	require.NoError(t, err)
	return tb
}

func NewTrustBaseFromVerifiers(t *testing.T, verifiers map[string]abcrypto.Verifier) types.RootTrustBase {
	var nodes []*types.NodeInfo
	for nodeID, v := range verifiers {
		nodes = append(nodes, NewNodeInfoFromVerifier(t, nodeID, v))
	}
	tb, err := types.NewTrustBaseGenesis(5, nodes)
	require.NoError(t, err)
	return tb
}

func NewNodeInfoFromVerifier(t *testing.T, nodeID string, sigVerifier abcrypto.Verifier) *types.NodeInfo {
	sigKey, err := sigVerifier.MarshalPublicKey()
	require.NoError(t, err)
	return &types.NodeInfo{
		NodeID: nodeID,
		SigKey: sigKey,
		Stake:  1,
	}
}

func NewAlwaysValidTrustBase(t *testing.T) types.RootTrustBase {
	return &AlwaysValidTrustBase{}
}

func (a AlwaysValidTrustBase) VerifyQuorumSignatures(data []byte, signatures map[string]hex.Bytes) error {
	return nil
}

func (a AlwaysValidTrustBase) VerifySignature(data []byte, sig []byte, nodeID string) (uint64, error) {
	return 1, nil
}

func (a AlwaysValidTrustBase) GetNetworkID() types.NetworkID {
	return types.NetworkLocal
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
